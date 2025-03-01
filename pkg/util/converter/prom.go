package converter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana/pkg/util/converter/jsonitere"
	jsoniter "github.com/json-iterator/go"
)

// helpful while debugging all the options that may appear
func logf(format string, a ...interface{}) {
	//fmt.Printf(format, a...)
}

type Options struct {
	Dataplane bool
}

func rspErr(e error) backend.DataResponse {
	return backend.DataResponse{Error: e}
}

// ReadPrometheusStyleResult will read results from a prometheus or loki server and return data frames
func ReadPrometheusStyleResult(jIter *jsoniter.Iterator, opt Options) backend.DataResponse {
	iter := jsonitere.NewIterator(jIter)
	var rsp backend.DataResponse
	status := "unknown"
	errorType := ""
	promErrString := ""
	warnings := []data.Notice{}

l1Fields:
	for l1Field, err := iter.ReadObject(); ; l1Field, err = iter.ReadObject() {
		if err != nil {
			return rspErr(err)
		}
		switch l1Field {
		case "status":
			if status, err = iter.ReadString(); err != nil {
				return rspErr(err)
			}

		case "data":
			rsp = readPrometheusData(iter, opt)
			if rsp.Error != nil {
				return rsp
			}

		case "error":
			if promErrString, err = iter.ReadString(); err != nil {
				return rspErr(err)
			}

		case "errorType":
			if errorType, err = iter.ReadString(); err != nil {
				return rspErr(err)
			}

		case "warnings":
			if warnings, err = readWarnings(iter); err != nil {
				return rspErr(err)
			}

		case "":
			if err != nil {
				return rspErr(err)
			}
			break l1Fields

		default:
			v, err := iter.Read()
			if err != nil {
				rsp.Error = err
				return rsp
			}
			logf("[ROOT] TODO, support key: %s / %v\n", l1Field, v)
		}
	}

	if status == "error" {
		return backend.DataResponse{
			Error: fmt.Errorf("%s: %s", errorType, promErrString),
		}
	}

	if len(warnings) > 0 {
		for _, frame := range rsp.Frames {
			if frame.Meta == nil {
				frame.Meta = &data.FrameMeta{}
			}
			frame.Meta.Notices = warnings
		}
	}

	return rsp
}

func readWarnings(iter *jsonitere.Iterator) ([]data.Notice, error) {
	warnings := []data.Notice{}
	next, err := iter.WhatIsNext()
	if err != nil {
		return nil, err
	}

	if next != jsoniter.ArrayValue {
		return warnings, nil
	}

	for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
		if err != nil {
			return nil, err
		}
		next, err := iter.WhatIsNext()
		if err != nil {
			return nil, err
		}
		if next == jsoniter.StringValue {
			s, err := iter.ReadString()
			if err != nil {
				return nil, err
			}
			notice := data.Notice{
				Severity: data.NoticeSeverityWarning,
				Text:     s,
			}
			warnings = append(warnings, notice)
		}
	}

	return warnings, nil
}

func readPrometheusData(iter *jsonitere.Iterator, opt Options) backend.DataResponse {
	var rsp backend.DataResponse
	t, err := iter.WhatIsNext()
	if err != nil {
		return rspErr(err)
	}

	if t == jsoniter.ArrayValue {
		return readArrayData(iter)
	}

	if t != jsoniter.ObjectValue {
		return backend.DataResponse{
			Error: fmt.Errorf("expected object type"),
		}
	}

	resultType := ""

l1Fields:
	for l1Field, err := iter.ReadObject(); ; l1Field, err = iter.ReadObject() {
		if err != nil {
			return rspErr(err)
		}
		switch l1Field {
		case "resultType":
			resultType, err = iter.ReadString()
			if err != nil {
				return rspErr(err)
			}
		case "result":
			switch resultType {
			case "matrix", "vector":
				rsp = readMatrixOrVectorMulti(iter, resultType, opt)
				if rsp.Error != nil {
					return rsp
				}
			case "streams":
				rsp = readStream(iter)
				if rsp.Error != nil {
					return rsp
				}
			case "string":
				rsp = readString(iter)
				if rsp.Error != nil {
					return rsp
				}
			case "scalar":
				rsp = readScalar(iter)
				if rsp.Error != nil {
					return rsp
				}
			default:
				if err = iter.Skip(); err != nil {
					return rspErr(err)
				}
				rsp = backend.DataResponse{
					Error: fmt.Errorf("unknown result type: %s", resultType),
				}
			}

		case "stats":
			v, err := iter.Read()
			if err != nil {
				rspErr(err)
			}
			if len(rsp.Frames) > 0 {
				meta := rsp.Frames[0].Meta
				if meta == nil {
					meta = &data.FrameMeta{}
					rsp.Frames[0].Meta = meta
				}
				meta.Custom = map[string]interface{}{
					"stats": v,
				}
			}

		case "":
			if err != nil {
				return rspErr(err)
			}
			break l1Fields

		default:
			v, err := iter.Read()
			if err != nil {
				return rspErr(err)
			}
			logf("[data] TODO, support key: %s / %v\n", l1Field, v)
		}
	}

	return rsp
}

// will return strings or exemplars
func readArrayData(iter *jsonitere.Iterator) backend.DataResponse {
	lookup := make(map[string]*data.Field)

	var labelFrame *data.Frame
	rsp := backend.DataResponse{}

	stringField := data.NewFieldFromFieldType(data.FieldTypeString, 0)
	stringField.Name = "Value"
	for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
		if err != nil {
			rspErr(err)
		}

		next, err := iter.WhatIsNext()
		if err != nil {
			return rspErr(err)
		}

		switch next {
		case jsoniter.StringValue:
			s, err := iter.ReadString()
			if err != nil {
				return rspErr(err)
			}
			stringField.Append(s)

		// Either label or exemplars
		case jsoniter.ObjectValue:
			exemplar, labelPairs, err := readLabelsOrExemplars(iter)
			if err != nil {
				rspErr(err)
			}
			if exemplar != nil {
				rsp.Frames = append(rsp.Frames, exemplar)
			} else if labelPairs != nil {
				max := 0
				for _, pair := range labelPairs {
					k := pair[0]
					v := pair[1]
					f, ok := lookup[k]
					if !ok {
						f = data.NewFieldFromFieldType(data.FieldTypeString, 0)
						f.Name = k
						lookup[k] = f

						if labelFrame == nil {
							labelFrame = data.NewFrame("")
							rsp.Frames = append(rsp.Frames, labelFrame)
						}
						labelFrame.Fields = append(labelFrame.Fields, f)
					}
					f.Append(fmt.Sprintf("%v", v))
					if f.Len() > max {
						max = f.Len()
					}
				}

				// Make sure all fields have equal length
				for _, f := range lookup {
					diff := max - f.Len()
					if diff > 0 {
						f.Extend(diff)
					}
				}
			}

		default:
			{
				ext, err := iter.ReadAny()
				if err != nil {
					rspErr(err)
				}
				v := fmt.Sprintf("%v", ext)
				stringField.Append(v)
			}
		}
	}

	if stringField.Len() > 0 {
		rsp.Frames = append(rsp.Frames, data.NewFrame("", stringField))
	}

	return rsp
}

// For consistent ordering read values to an array not a map
func readLabelsAsPairs(iter *jsonitere.Iterator) ([][2]string, error) {
	pairs := make([][2]string, 0, 10)
	for k, err := iter.ReadObject(); k != ""; k, err = iter.ReadObject() {
		if err != nil {
			return nil, err
		}
		v, err := iter.ReadString()
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]string{k, v})
	}
	return pairs, nil
}

func readLabelsOrExemplars(iter *jsonitere.Iterator) (*data.Frame, [][2]string, error) {
	pairs := make([][2]string, 0, 10)
	labels := data.Labels{}
	var frame *data.Frame

l1Fields:
	for l1Field, err := iter.ReadObject(); ; l1Field, err = iter.ReadObject() {
		if err != nil {
			return nil, nil, err
		}
		switch l1Field {
		case "seriesLabels":
			err = iter.ReadVal(&labels)
			if err != nil {
				return nil, nil, err
			}

		case "exemplars":
			lookup := make(map[string]*data.Field)
			timeField := data.NewFieldFromFieldType(data.FieldTypeTime, 0)
			timeField.Name = data.TimeSeriesTimeFieldName
			valueField := data.NewFieldFromFieldType(data.FieldTypeFloat64, 0)
			valueField.Name = data.TimeSeriesValueFieldName
			valueField.Labels = labels
			frame = data.NewFrame("", timeField, valueField)
			frame.Meta = &data.FrameMeta{
				Custom: resultTypeToCustomMeta("exemplar"),
			}
			for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
				if err != nil {
					return nil, nil, err
				}
				for l2Field, err := iter.ReadObject(); l2Field != ""; l2Field, err = iter.ReadObject() {
					if err != nil {
						return nil, nil, err
					}
					switch l2Field {
					// nolint:goconst
					case "value":
						s, err := iter.ReadString()
						if err != nil {
							return nil, nil, err
						}
						v, err := strconv.ParseFloat(s, 64)
						if err != nil {
							return nil, nil, err
						}
						valueField.Append(v)

					case "timestamp":
						f, err := iter.ReadFloat64()
						if err != nil {
							return nil, nil, err
						}
						ts := timeFromFloat(f)
						timeField.Append(ts)

					case "labels":
						max := 0
						pairs, err := readLabelsAsPairs(iter)
						if err != nil {
							return nil, nil, err
						}
						for _, pair := range pairs {
							k := pair[0]
							v := pair[1]
							f, ok := lookup[k]
							if !ok {
								f = data.NewFieldFromFieldType(data.FieldTypeString, 0)
								f.Name = k
								lookup[k] = f
								frame.Fields = append(frame.Fields, f)
							}
							f.Append(v)
							if f.Len() > max {
								max = f.Len()
							}
						}

						// Make sure all fields have equal length
						for _, f := range lookup {
							diff := max - f.Len()
							if diff > 0 {
								f.Extend(diff)
							}
						}

					default:
						if err = iter.Skip(); err != nil {
							return nil, nil, err
						}

						frame.AppendNotices(data.Notice{
							Severity: data.NoticeSeverityError,
							Text:     fmt.Sprintf("unable to parse key: %s in response body", l2Field),
						})
					}
				}
			}
		case "":
			if err != nil {
				return nil, nil, err
			}
			break l1Fields

		default:
			iV, err := iter.Read()
			if err != nil {
				return nil, nil, err
			}
			v := fmt.Sprintf("%v", iV)
			pairs = append(pairs, [2]string{l1Field, v})
		}
	}

	return frame, pairs, nil
}

func readString(iter *jsonitere.Iterator) backend.DataResponse {
	timeField := data.NewFieldFromFieldType(data.FieldTypeTime, 0)
	timeField.Name = data.TimeSeriesTimeFieldName
	valueField := data.NewFieldFromFieldType(data.FieldTypeString, 0)
	valueField.Name = data.TimeSeriesValueFieldName
	valueField.Labels = data.Labels{}

	_, err := iter.ReadArray()
	if err != nil {
		return rspErr(err)
	}

	var t float64
	if t, err = iter.ReadFloat64(); err != nil {
		return rspErr(err)
	}

	if _, err = iter.ReadArray(); err != nil {
		return rspErr(err)
	}

	var v string
	if v, err = iter.ReadString(); err != nil {
		return rspErr(err)
	}

	if _, err = iter.ReadArray(); err != nil {
		return rspErr(err)
	}

	tt := timeFromFloat(t)
	timeField.Append(tt)
	valueField.Append(v)

	frame := data.NewFrame("", timeField, valueField)
	frame.Meta = &data.FrameMeta{
		Type:   data.FrameTypeTimeSeriesMulti,
		Custom: resultTypeToCustomMeta("string"),
	}

	return backend.DataResponse{
		Frames: []*data.Frame{frame},
	}
}

func readScalar(iter *jsonitere.Iterator) backend.DataResponse {
	rsp := backend.DataResponse{}

	timeField := data.NewFieldFromFieldType(data.FieldTypeTime, 0)
	timeField.Name = data.TimeSeriesTimeFieldName
	valueField := data.NewFieldFromFieldType(data.FieldTypeFloat64, 0)
	valueField.Name = data.TimeSeriesValueFieldName
	valueField.Labels = data.Labels{}

	t, v, err := readTimeValuePair(iter)
	if err != nil {
		rsp.Error = err
		return rsp
	}
	timeField.Append(t)
	valueField.Append(v)

	frame := data.NewFrame("", timeField, valueField)
	frame.Meta = &data.FrameMeta{
		Type:   data.FrameTypeNumericMulti,
		Custom: resultTypeToCustomMeta("scalar"),
	}

	return backend.DataResponse{
		Frames: []*data.Frame{frame},
	}
}

func readMatrixOrVectorMulti(iter *jsonitere.Iterator, resultType string, opt Options) backend.DataResponse {
	rsp := backend.DataResponse{}

	for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
		if err != nil {
			return rspErr(err)
		}
		timeField := data.NewFieldFromFieldType(data.FieldTypeTime, 0)
		timeField.Name = data.TimeSeriesTimeFieldName
		valueField := data.NewFieldFromFieldType(data.FieldTypeFloat64, 0)
		valueField.Name = data.TimeSeriesValueFieldName
		valueField.Labels = data.Labels{}

		var histogram *histogramInfo

		for l1Field, err := iter.ReadObject(); l1Field != ""; l1Field, err = iter.ReadObject() {
			if err != nil {
				return rspErr(err)
			}
			switch l1Field {
			case "metric":
				if err = iter.ReadVal(&valueField.Labels); err != nil {
					return rspErr(err)
				}

			case "value":
				t, v, err := readTimeValuePair(iter)
				if err != nil {
					return rspErr(err)
				}
				timeField.Append(t)
				valueField.Append(v)

			// nolint:goconst
			case "values":
				for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
					if err != nil {
						return rspErr(err)
					}
					t, v, err := readTimeValuePair(iter)
					if err != nil {
						return rspErr(err)
					}
					timeField.Append(t)
					valueField.Append(v)
				}

			case "histogram":
				if histogram == nil {
					histogram = newHistogramInfo()
				}
				err = readHistogram(iter, histogram)
				if err != nil {
					return rspErr(err)
				}

			case "histograms":
				if histogram == nil {
					histogram = newHistogramInfo()
				}
				for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
					if err != nil {
						return rspErr(err)
					}
					if err = readHistogram(iter, histogram); err != nil {
						return rspErr(err)
					}
				}

			default:
				if err = iter.Skip(); err != nil {
					return rspErr(err)
				}
				logf("readMatrixOrVector: %s\n", l1Field)
			}
		}

		if histogram != nil {
			histogram.yMin.Labels = valueField.Labels
			frame := data.NewFrame(valueField.Name, histogram.time, histogram.yMin, histogram.yMax, histogram.count, histogram.yLayout)
			frame.Meta = &data.FrameMeta{
				Type: "heatmap-cells",
			}
			if frame.Name == data.TimeSeriesValueFieldName {
				frame.Name = "" // only set the name if useful
			}
			rsp.Frames = append(rsp.Frames, frame)
		} else {
			frame := data.NewFrame("", timeField, valueField)
			frame.Meta = &data.FrameMeta{
				Type:   data.FrameTypeTimeSeriesMulti,
				Custom: resultTypeToCustomMeta(resultType),
			}
			if opt.Dataplane && resultType == "vector" {
				frame.Meta.Type = data.FrameTypeNumericMulti
			}
			if opt.Dataplane {
				frame.Meta.TypeVersion = data.FrameTypeVersion{0, 1}
			}
			rsp.Frames = append(rsp.Frames, frame)
		}
	}

	return rsp
}

func readTimeValuePair(iter *jsonitere.Iterator) (time.Time, float64, error) {
	if _, err := iter.ReadArray(); err != nil {
		return time.Time{}, 0, err
	}

	t, err := iter.ReadFloat64()
	if err != nil {
		return time.Time{}, 0, err
	}

	if _, err = iter.ReadArray(); err != nil {
		return time.Time{}, 0, err
	}

	var v string
	if v, err = iter.ReadString(); err != nil {
		return time.Time{}, 0, err
	}

	if _, err = iter.ReadArray(); err != nil {
		return time.Time{}, 0, err
	}

	tt := timeFromFloat(t)
	fv, err := strconv.ParseFloat(v, 64)
	return tt, fv, err
}

type histogramInfo struct {
	//XMax (time)	YMin	Ymax	Count	YLayout
	time    *data.Field
	yMin    *data.Field // will have labels?
	yMax    *data.Field
	count   *data.Field
	yLayout *data.Field
}

func newHistogramInfo() *histogramInfo {
	hist := &histogramInfo{
		time:    data.NewFieldFromFieldType(data.FieldTypeTime, 0),
		yMin:    data.NewFieldFromFieldType(data.FieldTypeFloat64, 0),
		yMax:    data.NewFieldFromFieldType(data.FieldTypeFloat64, 0),
		count:   data.NewFieldFromFieldType(data.FieldTypeFloat64, 0),
		yLayout: data.NewFieldFromFieldType(data.FieldTypeInt8, 0),
	}
	hist.time.Name = "xMax"
	hist.yMin.Name = "yMin"
	hist.yMax.Name = "yMax"
	hist.count.Name = "count"
	hist.yLayout.Name = "yLayout"
	return hist
}

// This will read a single sparse histogram
// [ time, { count, sum, buckets: [...] }]
func readHistogram(iter *jsonitere.Iterator, hist *histogramInfo) error {
	// first element
	if _, err := iter.ReadArray(); err != nil {
		return err
	}

	f, err := iter.ReadFloat64()
	if err != nil {
		return err
	}
	t := timeFromFloat(f)

	// next object element
	if _, err := iter.ReadArray(); err != nil {
		return err
	}

	for l1Field, err := iter.ReadObject(); l1Field != ""; l1Field, err = iter.ReadObject() {
		if err != nil {
			return err
		}
		switch l1Field {
		case "count":
			if err = iter.Skip(); err != nil {
				return err
			}
		case "sum":
			if err = iter.Skip(); err != nil {
				return err
			}

		case "buckets":
			for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
				if err != nil {
					return err
				}
				hist.time.Append(t)

				if _, err := iter.ReadArray(); err != nil {
					return err
				}

				v, err := iter.ReadInt8()
				if err != nil {
					return err
				}
				hist.yLayout.Append(v)

				if _, err := iter.ReadArray(); err != nil {
					return err
				}

				if err = appendValueFromString(iter, hist.yMin); err != nil {
					return err
				}

				if _, err := iter.ReadArray(); err != nil {
					return err
				}

				err = appendValueFromString(iter, hist.yMax)
				if err != nil {
					return err
				}

				if _, err := iter.ReadArray(); err != nil {
					return err
				}

				err = appendValueFromString(iter, hist.count)
				if err != nil {
					return err
				}

				for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
					if err != nil {
						return err
					}
					return fmt.Errorf("expected close array")
				}
			}

		default:
			if err = iter.Skip(); err != nil {
				return err
			}
			logf("[SKIP]readHistogram: %s\n", l1Field)
		}
	}

	if more, err := iter.ReadArray(); more || err != nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("expected to be done")
	}

	return nil
}

func appendValueFromString(iter *jsonitere.Iterator, field *data.Field) error {
	var err error
	var s string
	if s, err = iter.ReadString(); err != nil {
		return err
	}

	var v float64
	if v, err = strconv.ParseFloat(s, 64); err != nil {
		return err
	}

	field.Append(v)
	return nil
}

func readStream(iter *jsonitere.Iterator) backend.DataResponse {
	rsp := backend.DataResponse{}

	labelsField := data.NewFieldFromFieldType(data.FieldTypeJSON, 0)
	labelsField.Name = "__labels" // avoid automatically spreading this by labels

	timeField := data.NewFieldFromFieldType(data.FieldTypeTime, 0)
	timeField.Name = "Time"

	lineField := data.NewFieldFromFieldType(data.FieldTypeString, 0)
	lineField.Name = "Line"

	// Nanoseconds time field
	tsField := data.NewFieldFromFieldType(data.FieldTypeString, 0)
	tsField.Name = "TS"

	labels := data.Labels{}
	labelJson, err := labelsToRawJson(labels)
	if err != nil {
		return backend.DataResponse{Error: err}
	}

	for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
		if err != nil {
			rspErr(err)
		}

	l1Fields:
		for l1Field, err := iter.ReadObject(); ; l1Field, err = iter.ReadObject() {
			if err != nil {
				return rspErr(err)
			}
			switch l1Field {
			case "stream":
				// we need to clear `labels`, because `iter.ReadVal`
				// only appends to it
				labels := data.Labels{}
				if err = iter.ReadVal(&labels); err != nil {
					return rspErr(err)
				}

				if labelJson, err = labelsToRawJson(labels); err != nil {
					return rspErr(err)
				}

			case "values":
				for more, err := iter.ReadArray(); more; more, err = iter.ReadArray() {
					if err != nil {
						rsp.Error = err
						return rsp
					}

					if _, err = iter.ReadArray(); err != nil {
						return rspErr(err)
					}

					ts, err := iter.ReadString()
					if err != nil {
						return rspErr(err)
					}

					if _, err = iter.ReadArray(); err != nil {
						return rspErr(err)
					}

					line, err := iter.ReadString()
					if err != nil {
						return rspErr(err)
					}

					if _, err = iter.ReadArray(); err != nil {
						return rspErr(err)
					}

					t, err := timeFromLokiString(ts)
					if err != nil {
						return rspErr(err)
					}

					labelsField.Append(labelJson)
					timeField.Append(t)
					lineField.Append(line)
					tsField.Append(ts)
				}
			case "":
				if err != nil {
					return rspErr(err)
				}
				break l1Fields
			}
		}
	}

	frame := data.NewFrame("", labelsField, timeField, lineField, tsField)
	frame.Meta = &data.FrameMeta{}
	rsp.Frames = append(rsp.Frames, frame)

	return rsp
}

func resultTypeToCustomMeta(resultType string) map[string]string {
	return map[string]string{"resultType": resultType}
}

func timeFromFloat(fv float64) time.Time {
	return time.UnixMilli(int64(fv * 1000.0)).UTC()
}

func timeFromLokiString(str string) (time.Time, error) {
	// normal time values look like: 1645030246277587968
	// and are less than: math.MaxInt65=9223372036854775807
	// This will do a fast path for any date before 2033
	s := len(str)
	if s < 19 || (s == 19 && str[0] == '1') {
		ns, err := strconv.ParseInt(str, 10, 64)
		if err == nil {
			return time.Unix(0, ns).UTC(), nil
		}
	}

	if s < 10 {
		return time.Time{}, fmt.Errorf("unexpected time format '%v' in response. response may have been truncated", str)
	}

	ss, _ := strconv.ParseInt(str[0:10], 10, 64)
	ns, _ := strconv.ParseInt(str[10:], 10, 64)
	return time.Unix(ss, ns).UTC(), nil
}

func labelsToRawJson(labels data.Labels) (json.RawMessage, error) {
	// data.Labels when converted to JSON keep the fields sorted
	bytes, err := jsoniter.Marshal(labels)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(bytes), nil
}
