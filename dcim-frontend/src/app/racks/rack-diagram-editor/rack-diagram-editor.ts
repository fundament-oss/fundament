import {
  ChangeDetectionStrategy,
  Component,
  computed,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
} from '@angular/core';
import {
  CdkDrag,
  CdkDragDrop,
  CdkDragPreview,
  CdkDropList,
  moveItemInArray,
} from '@angular/cdk/drag-drop';
import { RackDevice, Rack } from '../rack.model';
import RackDiagramComponent from '../rack-diagram/rack-diagram';

type EditorRow = { type: 'device'; device: RackDevice } | { type: 'empty'; u: number };

@Component({
  selector: 'app-rack-diagram-editor',
  templateUrl: './rack-diagram-editor.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CdkDropList, CdkDrag, CdkDragPreview],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
})
export default class RackDiagramEditorComponent {
  readonly rack = input.required<Rack>();

  readonly devicesChange = output<RackDevice[]>();

  readonly deleteDeviceRequest = output<RackDevice>();

  readonly editorRows = computed((): EditorRow[] => {
    const rack = this.rack();
    const slotMap = new Map<number, RackDevice>();
    rack.devices.forEach((dev) => {
      for (let u = dev.uStart; u < dev.uStart + dev.uSize; u += 1) {
        slotMap.set(u, dev);
      }
    });
    const rows: EditorRow[] = [];
    const seen = new Set<string>();
    for (let u = rack.totalU; u >= 1; u -= 1) {
      const dev = slotMap.get(u) ?? null;
      if (dev) {
        if (!seen.has(dev.id)) {
          seen.add(dev.id);
          rows.push({ type: 'device', device: dev });
        }
      } else {
        rows.push({ type: 'empty', u });
      }
    }
    return rows;
  });

  readonly uNumbers = computed((): number[] => {
    const result: number[] = [];
    this.editorRows().forEach((row) => {
      if (row.type === 'device') {
        for (let i = 0; i < row.device.uSize; i += 1) {
          result.push(row.device.uStart + row.device.uSize - 1 - i);
        }
      } else {
        result.push(row.u);
      }
    });
    return result;
  });

  readonly deviceSlotClasses = RackDiagramComponent.deviceSlotClasses;

  readonly deviceHeight = (device: RackDevice): number => device.uSize * 28 + (device.uSize - 1);

  onDrop(event: CdkDragDrop<EditorRow[]>): void {
    const rows = [...this.editorRows()];
    moveItemInArray(rows, event.previousIndex, event.currentIndex);
    // Walk rows top-to-bottom and assign U positions directly, preserving gaps.
    // Empty rows consume 1U; device rows consume their uSize.
    let currentU = this.rack().totalU;
    const updatedDevices: RackDevice[] = [];
    rows.forEach((row) => {
      if (row.type === 'device') {
        updatedDevices.push({ ...row.device, uStart: currentU - row.device.uSize + 1 });
        currentU -= row.device.uSize;
      } else {
        currentU -= 1;
      }
    });
    this.devicesChange.emit(updatedDevices);
  }

  requestDelete(device: RackDevice): void {
    this.deleteDeviceRequest.emit(device);
  }

}
