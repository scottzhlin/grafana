import React, { useCallback, useState } from 'react';

import { DataFrame, EventBus, AbsoluteTimeRange, TimeZone, SplitOpen, LoadingState } from '@grafana/data';
import { PanelChrome } from '@grafana/ui';
import { ExploreGraphStyle } from 'app/types';

import { storeGraphStyle } from '../state/utils';

import { ExploreGraph } from './ExploreGraph';
import { ExploreGraphLabel } from './ExploreGraphLabel';
import { loadGraphStyle } from './utils';

interface Props {
  loading: boolean;
  data: DataFrame[];
  annotations?: DataFrame[];
  eventBus: EventBus;
  height: number;
  width: number;
  absoluteRange: AbsoluteTimeRange;
  timeZone: TimeZone;
  onChangeTime: (absoluteRange: AbsoluteTimeRange) => void;
  splitOpenFn: SplitOpen;
  loadingState: LoadingState;
}

export const GraphContainer = ({
  data,
  eventBus,
  height,
  width,
  absoluteRange,
  timeZone,
  annotations,
  onChangeTime,
  splitOpenFn,
  loadingState,
}: Props) => {
  const [graphStyle, setGraphStyle] = useState(loadGraphStyle);

  const onGraphStyleChange = useCallback((graphStyle: ExploreGraphStyle) => {
    storeGraphStyle(graphStyle);
    setGraphStyle(graphStyle);
  }, []);

  return (
    <PanelChrome
      title="Graph"
      width={width}
      height={height}
      loadingState={loadingState}
      actions={<ExploreGraphLabel graphStyle={graphStyle} onChangeGraphStyle={onGraphStyleChange} />}
    >
      {(innerWidth, innerHeight) => (
        <ExploreGraph
          graphStyle={graphStyle}
          data={data}
          height={innerHeight}
          width={innerWidth}
          absoluteRange={absoluteRange}
          onChangeTime={onChangeTime}
          timeZone={timeZone}
          annotations={annotations}
          splitOpenFn={splitOpenFn}
          loadingState={loadingState}
          eventBus={eventBus}
        />
      )}
    </PanelChrome>
  );
};
