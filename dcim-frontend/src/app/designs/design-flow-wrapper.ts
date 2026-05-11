import {
  AfterViewInit,
  Component,
  ElementRef,
  Input,
  OnChanges,
  OnDestroy,
  output,
  ViewChild,
  ViewEncapsulation,
} from '@angular/core';
import * as React from 'react';
import * as ReactDOM from 'react-dom/client';
import { DesignFlow, DesignFlowProps } from './design-flow';
import { LogicalDevice, LogicalConnection, LogicalDeviceLayout } from './design.model';

@Component({
  selector: 'app-design-flow',
  template: `<div #container class="w-full h-full"></div>`,
  styleUrls: ['../../../node_modules/reactflow/dist/style.css'],
  encapsulation: ViewEncapsulation.None,
})
export default class DesignFlowWrapperComponent implements AfterViewInit, OnChanges, OnDestroy {
  @ViewChild('container', { static: true }) container!: ElementRef;

  @Input() devices: LogicalDevice[] = [];

  @Input() connections: LogicalConnection[] = [];

  @Input() layouts: LogicalDeviceLayout[] = [];

  @Input() selectedDeviceId: string | null = null;

  readonly deviceSelected = output<string | null>();

  readonly layoutChanged = output<LogicalDeviceLayout[]>();

  private root: ReturnType<typeof ReactDOM.createRoot> | undefined;

  ngAfterViewInit() {
    this.root = ReactDOM.createRoot(this.container.nativeElement);
    this.render();
  }

  ngOnChanges() {
    if (this.root) this.render();
  }

  ngOnDestroy() {
    if (this.root) this.root.unmount();
  }

  private render() {
    const props: DesignFlowProps = {
      devices: this.devices,
      connections: this.connections,
      layouts: this.layouts,
      selectedDeviceId: this.selectedDeviceId,
      onSelectDevice: (id) => this.deviceSelected.emit(id),
      onLayoutChange: (layouts) => this.layoutChanged.emit(layouts),
    };
    this.root!.render(React.createElement(DesignFlow, props));
  }
}
