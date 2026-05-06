import {
  Component,
  EventEmitter,
  Input,
  Output,
  ViewEncapsulation,
  AfterViewInit,
  OnDestroy,
  ElementRef,
  ViewChild,
  OnChanges,
  SimpleChanges,
} from '@angular/core';
import * as React from 'react';
import * as ReactDOM from 'react-dom/client';
import {
  Node,
  Connection,
  NodeChange,
  EdgeChange,
  OnConnectStartParams,
  ReactFlowInstance,
  OnSelectionChangeParams,
  Edge,
  DefaultEdgeOptions,
  HandleType,
  NodeTypes,
  EdgeTypes,
  ConnectionLineType,
  ConnectionLineComponent,
  ConnectionMode,
  KeyCode,
  NodeOrigin,
  Viewport,
  CoordinateExtent,
  PanOnScrollMode,
  FitViewOptions,
  PanelPosition,
  ProOptions,
  OnError,
} from 'reactflow';
import { ReactFlowWrappableComponent } from './reactflow';

@Component({
  selector: 'app-ngx-reactflow',
  template: `<div #reactContainer class="w-full h-full"></div>`,
  styleUrls: ['../../../node_modules/reactflow/dist/style.css'],
  encapsulation: ViewEncapsulation.None,
})
export default class ReactFlowComponent implements AfterViewInit, OnDestroy, OnChanges {
  @ViewChild('reactContainer', { static: true }) container!: ElementRef;

  private root: ReturnType<typeof ReactDOM.createRoot> | undefined;

  ngReactComponent = ReactFlowWrappableComponent;

  @Input() nodes?: Node<Record<string, unknown>, string | undefined>[] | undefined;

  @Input() edges?: Edge<Record<string, unknown>>[] | undefined;

  @Input() defaultEdgeOptions?: DefaultEdgeOptions | undefined;

  @Output() nodeClick = new EventEmitter<[MouseEvent, Node]>();

  @Output() nodeDoubleClick = new EventEmitter<[MouseEvent, Node]>();

  @Output() nodeMouseEnter = new EventEmitter<[MouseEvent, Node]>();

  @Output() nodeMouseMove = new EventEmitter<[MouseEvent, Node]>();

  @Output() nodeMouseLeave = new EventEmitter<[MouseEvent, Node]>();

  @Output() nodeContextMenu = new EventEmitter<[MouseEvent, Node]>();

  @Output() nodeDragStart = new EventEmitter<[MouseEvent, Node, Node[]]>();

  @Output() nodeDrag = new EventEmitter<[MouseEvent, Node, Node[]]>();

  @Output() nodeDragStop = new EventEmitter<[MouseEvent, Node, Node[]]>();

  @Output() edgeClick = new EventEmitter<[MouseEvent, Edge]>();

  @Output() edgeUpdate = new EventEmitter<[Edge<Record<string, unknown>>, Connection]>();

  @Output() edgeContextMenu = new EventEmitter<[MouseEvent, Edge]>();

  @Output() edgeMouseEnter = new EventEmitter<[MouseEvent, Edge]>();

  @Output() edgeMouseMove = new EventEmitter<[MouseEvent, Edge]>();

  @Output() edgeMouseLeave = new EventEmitter<[MouseEvent, Edge]>();

  @Output() edgeDoubleClick = new EventEmitter<[MouseEvent, Edge]>();

  @Output() edgeUpdateStart = new EventEmitter<
    [MouseEvent, Edge<Record<string, unknown>>, HandleType]
  >();

  @Output() edgeUpdateEnd = new EventEmitter<
    [MouseEvent, Edge<Record<string, unknown>>, HandleType]
  >();

  @Output() nodesChange = new EventEmitter<[NodeChange[]]>();

  @Output() edgesChange = new EventEmitter<[EdgeChange[]]>();

  @Output() nodesDelete = new EventEmitter<[Node[]]>();

  @Output() edgesDelete = new EventEmitter<[Edge[]]>();

  @Output() selectionDragStart = new EventEmitter<[MouseEvent, Node[]]>();

  @Output() selectionDrag = new EventEmitter<[MouseEvent, Node[]]>();

  @Output() selectionDragStop = new EventEmitter<[MouseEvent, Node[]]>();

  @Output() selectionStart = new EventEmitter<[MouseEvent]>();

  @Output() selectionEnd = new EventEmitter<[MouseEvent]>();

  @Output() selectionContextMenu = new EventEmitter<
    [MouseEvent, Node<Record<string, unknown>, string | undefined>[]]
  >();

  @Output() connect = new EventEmitter<[Connection]>();

  @Output() connectStart = new EventEmitter<[MouseEvent, OnConnectStartParams]>();

  @Output() connectEnd = new EventEmitter<[MouseEvent]>();

  @Output() clickConnectStart = new EventEmitter<[MouseEvent, OnConnectStartParams]>();

  @Output() clickConnectEnd = new EventEmitter<[MouseEvent]>();

  @Output() init = new EventEmitter<
    [ReactFlowInstance<Record<string, unknown>, Record<string, unknown>>]
  >();

  @Output() move = new EventEmitter<[MouseEvent, Viewport]>();

  @Output() moveStart = new EventEmitter<[MouseEvent, Viewport]>();

  @Output() moveEnd = new EventEmitter<[MouseEvent, Viewport]>();

  @Output() selectionChange = new EventEmitter<[OnSelectionChangeParams]>();

  @Output() paneScroll = new EventEmitter<[WheelEvent]>();

  @Output() paneClick = new EventEmitter<[MouseEvent]>();

  @Output() paneContextMenu = new EventEmitter<[MouseEvent]>();

  @Output() paneMouseEnter = new EventEmitter<[MouseEvent]>();

  @Output() paneMouseMove = new EventEmitter<[MouseEvent]>();

  @Output() paneMouseLeave = new EventEmitter<[MouseEvent]>();

  @Output() flowError = new EventEmitter<OnError>();

  @Input() nodeTypes?: NodeTypes | undefined;

  @Input() edgeTypes?: EdgeTypes | undefined;

  @Input() connectionLineType?: ConnectionLineType | undefined;

  @Input() connectionLineStyle?: React.CSSProperties | undefined;

  @Input() connectionLineComponent?: ConnectionLineComponent | undefined;

  @Input() connectionLineContainerStyle?: React.CSSProperties | undefined;

  @Input() connectionMode?: ConnectionMode | undefined;

  @Input() deleteKeyCode?: KeyCode | null | undefined;

  @Input() selectionKeyCode?: KeyCode | null | undefined;

  @Input() selectionOnDrag?: boolean | undefined;

  @Input() panActivationKeyCode?: KeyCode | null | undefined;

  @Input() multiSelectionKeyCode?: KeyCode | null | undefined;

  @Input() zoomActivationKeyCode?: KeyCode | null | undefined;

  @Input() snapToGrid?: boolean | undefined;

  @Input() snapGrid?: [number, number] | undefined;

  @Input() onlyRenderVisibleElements?: boolean | undefined;

  @Input() nodesDraggable?: boolean | undefined;

  @Input() nodesConnectable?: boolean | undefined;

  @Input() nodesFocusable?: boolean | undefined;

  @Input() nodeOrigin?: NodeOrigin | undefined;

  @Input() edgesFocusable?: boolean | undefined;

  @Input() elementsSelectable?: boolean | undefined;

  @Input() selectNodesOnDrag?: boolean | undefined;

  @Input() panOnDrag?: boolean | number[] | undefined;

  @Input() minZoom?: number | undefined;

  @Input() maxZoom?: number | undefined;

  @Input() defaultViewport?: Viewport | undefined;

  @Input() translateExtent?: CoordinateExtent | undefined;

  @Input() preventScrolling?: boolean | undefined;

  @Input() nodeExtent?: CoordinateExtent | undefined;

  @Input() defaultMarkerColor?: string | undefined;

  @Input() zoomOnScroll?: boolean | undefined;

  @Input() zoomOnPinch?: boolean | undefined;

  @Input() panOnScroll?: boolean | undefined;

  @Input() panOnScrollSpeed?: number | undefined;

  @Input() panOnScrollMode?: PanOnScrollMode | undefined;

  @Input() zoomOnDoubleClick?: boolean | undefined;

  @Input() edgeUpdaterRadius?: number | undefined;

  @Input() noDragClassName?: string | undefined;

  @Input() noWheelClassName?: string | undefined;

  @Input() noPanClassName?: string | undefined;

  @Input() fitView?: boolean | undefined;

  @Input() fitViewOptions?: FitViewOptions | undefined;

  @Input() connectOnClick?: boolean | undefined;

  @Input() attributionPosition?: PanelPosition | undefined;

  @Input() proOptions?: ProOptions | undefined;

  @Input() elevateNodesOnSelect?: boolean | undefined;

  @Input() elevateEdgesOnSelect?: boolean | undefined;

  @Input() disableKeyboardA11y?: boolean | undefined;

  @Input() autoPanOnNodeDrag?: boolean | undefined;

  @Input() autoPanOnConnect?: boolean | undefined;

  @Input() connectionRadius?: number | undefined;

  ngAfterViewInit() {
    const props = this.getProps();
    this.root = ReactDOM.createRoot(this.container.nativeElement);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    this.root.render(React.createElement(this.ngReactComponent, props as any));
  }

  ngOnChanges(_changes: SimpleChanges) {
    if (this.root) {
      const props = this.getProps();
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      this.root.render(React.createElement(this.ngReactComponent, props as any));
    }
  }

  ngOnDestroy() {
    if (this.root) {
      this.root.unmount();
    }
  }

  private getProps() {
    return {
      nodes: this.nodes,
      edges: this.edges,
      defaultEdgeOptions: this.defaultEdgeOptions,
      nodeTypes: this.nodeTypes,
      edgeTypes: this.edgeTypes,
      connectionLineType: this.connectionLineType,
      connectionLineStyle: this.connectionLineStyle,
      connectionLineComponent: this.connectionLineComponent,
      connectionLineContainerStyle: this.connectionLineContainerStyle,
      connectionMode: this.connectionMode,
      deleteKeyCode: this.deleteKeyCode,
      selectionKeyCode: this.selectionKeyCode,
      selectionOnDrag: this.selectionOnDrag,
      panActivationKeyCode: this.panActivationKeyCode,
      multiSelectionKeyCode: this.multiSelectionKeyCode,
      zoomActivationKeyCode: this.zoomActivationKeyCode,
      snapToGrid: this.snapToGrid,
      snapGrid: this.snapGrid,
      onlyRenderVisibleElements: this.onlyRenderVisibleElements,
      nodesDraggable: this.nodesDraggable,
      nodesConnectable: this.nodesConnectable,
      nodesFocusable: this.nodesFocusable,
      nodeOrigin: this.nodeOrigin,
      edgesFocusable: this.edgesFocusable,
      elementsSelectable: this.elementsSelectable,
      selectNodesOnDrag: this.selectNodesOnDrag,
      panOnDrag: this.panOnDrag,
      minZoom: this.minZoom,
      maxZoom: this.maxZoom,
      defaultViewport: this.defaultViewport,
      translateExtent: this.translateExtent,
      preventScrolling: this.preventScrolling,
      nodeExtent: this.nodeExtent,
      defaultMarkerColor: this.defaultMarkerColor,
      zoomOnScroll: this.zoomOnScroll,
      zoomOnPinch: this.zoomOnPinch,
      panOnScroll: this.panOnScroll,
      panOnScrollSpeed: this.panOnScrollSpeed,
      panOnScrollMode: this.panOnScrollMode,
      zoomOnDoubleClick: this.zoomOnDoubleClick,
      edgeUpdaterRadius: this.edgeUpdaterRadius,
      noDragClassName: this.noDragClassName,
      noWheelClassName: this.noWheelClassName,
      noPanClassName: this.noPanClassName,
      fitView: this.fitView,
      fitViewOptions: this.fitViewOptions,
      connectOnClick: this.connectOnClick,
      attributionPosition: this.attributionPosition,
      proOptions: this.proOptions,
      elevateNodesOnSelect: this.elevateNodesOnSelect,
      elevateEdgesOnSelect: this.elevateEdgesOnSelect,
      disableKeyboardA11y: this.disableKeyboardA11y,
      autoPanOnNodeDrag: this.autoPanOnNodeDrag,
      autoPanOnConnect: this.autoPanOnConnect,
      connectionRadius: this.connectionRadius,
      onNodeClick: (event: React.MouseEvent, node: Node) =>
        this.nodeClick.emit([event as unknown as MouseEvent, node]),
      onNodeDoubleClick: (event: React.MouseEvent, node: Node) =>
        this.nodeDoubleClick.emit([event as unknown as MouseEvent, node]),
      onNodeMouseEnter: (event: React.MouseEvent, node: Node) =>
        this.nodeMouseEnter.emit([event as unknown as MouseEvent, node]),
      onNodeMouseMove: (event: React.MouseEvent, node: Node) =>
        this.nodeMouseMove.emit([event as unknown as MouseEvent, node]),
      onNodeMouseLeave: (event: React.MouseEvent, node: Node) =>
        this.nodeMouseLeave.emit([event as unknown as MouseEvent, node]),
      onNodeContextMenu: (event: React.MouseEvent, node: Node) =>
        this.nodeContextMenu.emit([event as unknown as MouseEvent, node]),
      onNodeDragStart: (event: React.MouseEvent, node: Node, nodes: Node[]) =>
        this.nodeDragStart.emit([event as unknown as MouseEvent, node, nodes]),
      onNodeDrag: (event: React.MouseEvent, node: Node, nodes: Node[]) =>
        this.nodeDrag.emit([event as unknown as MouseEvent, node, nodes]),
      onNodeDragStop: (event: React.MouseEvent, node: Node, nodes: Node[]) =>
        this.nodeDragStop.emit([event as unknown as MouseEvent, node, nodes]),
      onEdgeClick: (event: React.MouseEvent, edge: Edge) =>
        this.edgeClick.emit([event as unknown as MouseEvent, edge]),
      onEdgeUpdate: (oldEdge: Edge<Record<string, unknown>>, newConnection: Connection) =>
        this.edgeUpdate.emit([oldEdge, newConnection]),
      onEdgeContextMenu: (event: React.MouseEvent, edge: Edge) =>
        this.edgeContextMenu.emit([event as unknown as MouseEvent, edge]),
      onEdgeMouseEnter: (event: React.MouseEvent, edge: Edge) =>
        this.edgeMouseEnter.emit([event as unknown as MouseEvent, edge]),
      onEdgeMouseMove: (event: React.MouseEvent, edge: Edge) =>
        this.edgeMouseMove.emit([event as unknown as MouseEvent, edge]),
      onEdgeMouseLeave: (event: React.MouseEvent, edge: Edge) =>
        this.edgeMouseLeave.emit([event as unknown as MouseEvent, edge]),
      onEdgeDoubleClick: (event: React.MouseEvent, edge: Edge) =>
        this.edgeDoubleClick.emit([event as unknown as MouseEvent, edge]),
      onEdgeUpdateStart: (event: React.MouseEvent, edge: Edge, handleType: HandleType) =>
        this.edgeUpdateStart.emit([event as unknown as MouseEvent, edge, handleType]),
      onEdgeUpdateEnd: (event: React.MouseEvent, edge: Edge, handleType: HandleType) =>
        this.edgeUpdateEnd.emit([event as unknown as MouseEvent, edge, handleType]),
      onNodesChange: (nodeChanges: NodeChange[]) => this.nodesChange.emit([nodeChanges]),
      onEdgesChange: (edgeChanges: EdgeChange[]) => this.edgesChange.emit([edgeChanges]),
      onNodesDelete: (nodes: Node[]) => this.nodesDelete.emit([nodes]),
      onEdgesDelete: (edges: Edge[]) => this.edgesDelete.emit([edges]),
      onSelectionDragStart: (event: React.MouseEvent, nodes: Node[]) =>
        this.selectionDragStart.emit([event as unknown as MouseEvent, nodes]),
      onSelectionDrag: (event: React.MouseEvent, nodes: Node[]) =>
        this.selectionDrag.emit([event as unknown as MouseEvent, nodes]),
      onSelectionDragStop: (event: React.MouseEvent, nodes: Node[]) =>
        this.selectionDragStop.emit([event as unknown as MouseEvent, nodes]),
      onSelectionStart: (event: React.MouseEvent) =>
        this.selectionStart.emit([event as unknown as MouseEvent]),
      onSelectionEnd: (event: React.MouseEvent) =>
        this.selectionEnd.emit([event as unknown as MouseEvent]),
      onSelectionContextMenu: (event: React.MouseEvent, nodes: Node[]) =>
        this.selectionContextMenu.emit([event as unknown as MouseEvent, nodes]),
      onConnect: (connection: Connection) => this.connect.emit([connection]),
      onConnectStart: (event: React.MouseEvent, params: OnConnectStartParams) =>
        this.connectStart.emit([event as unknown as MouseEvent, params]),
      onConnectEnd: (event: React.MouseEvent) =>
        this.connectEnd.emit([event as unknown as MouseEvent]),
      onClickConnectStart: (event: React.MouseEvent, params: OnConnectStartParams) =>
        this.clickConnectStart.emit([event as unknown as MouseEvent, params]),
      onClickConnectEnd: (event: React.MouseEvent) =>
        this.clickConnectEnd.emit([event as unknown as MouseEvent]),
      onInit: (instance: ReactFlowInstance) => this.init.emit([instance]),
      onMove: (event: React.MouseEvent, viewport: Viewport) =>
        this.move.emit([event as unknown as MouseEvent, viewport]),
      onMoveStart: (event: React.MouseEvent, viewport: Viewport) =>
        this.moveStart.emit([event as unknown as MouseEvent, viewport]),
      onMoveEnd: (event: React.MouseEvent, viewport: Viewport) =>
        this.moveEnd.emit([event as unknown as MouseEvent, viewport]),
      onSelectionChange: (params: OnSelectionChangeParams) => this.selectionChange.emit([params]),
      onPaneScroll: (event: React.WheelEvent) =>
        this.paneScroll.emit([event as unknown as WheelEvent]),
      onPaneClick: (event: React.MouseEvent) =>
        this.paneClick.emit([event as unknown as MouseEvent]),
      onPaneContextMenu: (event: React.MouseEvent) =>
        this.paneContextMenu.emit([event as unknown as MouseEvent]),
      onPaneMouseEnter: (event: React.MouseEvent) =>
        this.paneMouseEnter.emit([event as unknown as MouseEvent]),
      onPaneMouseMove: (event: React.MouseEvent) =>
        this.paneMouseMove.emit([event as unknown as MouseEvent]),
      onPaneMouseLeave: (event: React.MouseEvent) =>
        this.paneMouseLeave.emit([event as unknown as MouseEvent]),
      onError: (err: OnError) => this.flowError.emit(err),
    };
  }
}
