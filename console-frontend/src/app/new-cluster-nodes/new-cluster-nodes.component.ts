import {
  Component,
  inject,
  OnInit,
  signal,
  ViewChild,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';
import { NewClusterFormStateService } from '../new-cluster-form/new-cluster-form-state.service';
import { MachineTypeOption, RegionCatalogService } from '../region-catalog.service';

@Component({
  selector: 'app-new-cluster-nodes',
  imports: [SharedNodePoolsFormComponent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './new-cluster-nodes.component.html',
})
export default class NewClusterNodesComponent implements OnInit {
  @ViewChild(SharedNodePoolsFormComponent) nodePoolsForm!: SharedNodePoolsFormComponent;

  private stateService = inject(NewClusterFormStateService);

  private regionCatalog = inject(RegionCatalogService);

  // Machine types offered by the region chosen in step 1.
  machineTypeOptions = signal<MachineTypeOption[] | null>(null);

  async ngOnInit() {
    const { region: regionName } = this.stateService.getState();
    if (!regionName) {
      return;
    }
    try {
      const region = await this.regionCatalog.getRegionByName(regionName);
      if (region) {
        this.machineTypeOptions.set(RegionCatalogService.machineTypeOptions(region));
      }
    } catch {
      // Catalog unavailable: the form falls back to its built-in list.
    }
  }

  onFormSubmit(data: { nodePools: NodePoolData[] }) {
    // Save node pools to state
    this.stateService.updateNodePools(data.nodePools);
    this.stateService.markStepCompleted(1);

    this.stateService.goToStep(2);
  }
}
