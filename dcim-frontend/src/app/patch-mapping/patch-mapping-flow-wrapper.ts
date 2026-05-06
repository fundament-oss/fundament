import {
  AfterViewInit,
  Component,
  ElementRef,
  OnDestroy,
  ViewChild,
  ViewEncapsulation,
} from '@angular/core';
import * as React from 'react';
import * as ReactDOM from 'react-dom/client';
import { PatchMappingFlow } from './patch-mapping-flow';

@Component({
  selector: 'app-patch-mapping-flow',
  template: `<div #container class="w-full h-full"></div>`,
  styleUrls: ['../../../node_modules/reactflow/dist/style.css'],
  encapsulation: ViewEncapsulation.None,
})
export default class PatchMappingFlowWrapperComponent implements AfterViewInit, OnDestroy {
  @ViewChild('container', { static: true }) container!: ElementRef;

  private root: ReturnType<typeof ReactDOM.createRoot> | undefined;

  ngAfterViewInit() {
    this.root = ReactDOM.createRoot(this.container.nativeElement);
    this.root.render(React.createElement(PatchMappingFlow));
  }

  ngOnDestroy() {
    if (this.root) {
      this.root.unmount();
    }
  }
}
