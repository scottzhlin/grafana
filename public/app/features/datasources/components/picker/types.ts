import React from 'react';
import { DropzoneOptions } from 'react-dropzone';

import { DataSourceInstanceSettings } from '@grafana/data';
import { DataSourceJsonData, DataSourceRef } from '@grafana/schema';

export interface DataSourceDrawerProps {
  datasources: Array<DataSourceInstanceSettings<DataSourceJsonData>>;
  onChange: (ds: DataSourceInstanceSettings<DataSourceJsonData>) => void;
  current: DataSourceInstanceSettings<DataSourceJsonData> | string | DataSourceRef | null | undefined;
  enableFileUpload?: boolean;
  fileUploadOptions?: DropzoneOptions;
  onClickAddCSV?: () => void;
  recentlyUsed?: string[];
}

export interface PickerContentProps extends DataSourceDrawerProps {
  style: React.CSSProperties;
  filterTerm?: string;
  onClose: () => void;
  onDismiss: () => void;
}

export interface DataSourcePickerProps {
  onChange: (ds: DataSourceInstanceSettings) => void;
  current: DataSourceRef | string | null; // uid
  tracing?: boolean;
  recentlyUsed?: string[];
  mixed?: boolean;
  dashboard?: boolean;
  metrics?: boolean;
  type?: string | string[];
  annotations?: boolean;
  variables?: boolean;
  alerting?: boolean;
  pluginId?: string;
  /** If true,we show only DSs with logs; and if true, pluginId shouldnt be passed in */
  logs?: boolean;
  // Does not set the default data source if there is no value.
  noDefault?: boolean;
  inputId?: string;
  filter?: (dataSource: DataSourceInstanceSettings) => boolean;
  onClear?: () => void;
  disabled?: boolean;
  enableFileUpload?: boolean;
  fileUploadOptions?: DropzoneOptions;
  onClickAddCSV?: () => void;
}

export interface DataSourcePickerWithHistoryProps extends Omit<DataSourcePickerProps, 'recentlyUsed'> {
  localStorageKey?: string;
}

export interface DataSourcePickerHistoryItem {
  lastUse: string;
  uid: string;
}
