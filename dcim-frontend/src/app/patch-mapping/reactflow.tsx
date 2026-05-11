import React from 'react';
import ReactFlow, { ReactFlowProps } from 'reactflow';

export const ReactFlowWrappableComponent = (props: ReactFlowProps) => {
  return React.createElement(ReactFlow, props);
};
