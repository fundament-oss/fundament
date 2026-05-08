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
import { Cable, Port } from './cable.model';
import { PatchMappingFlow } from './patch-mapping-flow';

@Component({
  selector: 'app-patch-mapping-flow',
  template: `<div #container class="w-full h-full"></div>`,
  styleUrls: ['../../../node_modules/reactflow/dist/style.css'],
  encapsulation: ViewEncapsulation.None,
})
export default class PatchMappingFlowWrapperComponent
  implements AfterViewInit, OnChanges, OnDestroy
{
  @ViewChild('container', { static: true }) container!: ElementRef;

  @Input() cables: Cable[] = [];

  @Input() devicePorts: Record<string, Port[]> = {};

  @Input() selectedCableId: string | null = null;

  readonly cableSelected = output<string>();

  readonly deviceNavigate = output<string>();

  private root: ReturnType<typeof ReactDOM.createRoot> | undefined;

  ngAfterViewInit() {
    this.root = ReactDOM.createRoot(this.container.nativeElement);
    this.render();
  }

  ngOnChanges() {
    if (this.root) this.render();
  }

  ngOnDestroy() {
    this.root?.unmount();
  }

  private render() {
    this.root!.render(
      React.createElement(PatchMappingFlow, {
        cables: this.cables,
        devicePorts: this.devicePorts,
        selectedCableId: this.selectedCableId,
        onCableClick: (id: string) => this.cableSelected.emit(id),
        onDeviceClick: (id: string) => this.deviceNavigate.emit(id),
      }),
    );
  }
}
