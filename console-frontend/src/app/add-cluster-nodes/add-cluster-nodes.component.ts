import {
  Component,
  inject,
  OnInit,
  signal,
  ViewChild,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { TitleService } from '../title.service';
import {
  SharedNodePoolsFormComponent,
  NodePoolData,
} from '../shared-node-pools-form/shared-node-pools-form.component';
import { ClusterWizardStateService } from '../add-cluster-wizard-layout/cluster-wizard-state.service';
import { MachineTypeOption, RegionCatalogService } from '../region-catalog.service';

@Component({
  selector: 'app-add-cluster-nodes',
  imports: [SharedNodePoolsFormComponent, RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './add-cluster-nodes.component.html',
})
export default class AddClusterNodesComponent implements OnInit {
  @ViewChild(SharedNodePoolsFormComponent) nodePoolsForm!: SharedNodePoolsFormComponent;

  private titleService = inject(TitleService);

  private router = inject(Router);

  private stateService = inject(ClusterWizardStateService);

  private regionCatalog = inject(RegionCatalogService);

  // Machine types offered by the region chosen in step 1.
  machineTypeOptions = signal<MachineTypeOption[] | null>(null);

  constructor() {
    this.titleService.setTitle('Add cluster nodes');
  }

  async ngOnInit() {
    const { region: regionName } = this.stateService.getState();
    if (!regionName) {
      return;
    }
    const region = await this.regionCatalog.getRegionByName(regionName);
    if (region) {
      this.machineTypeOptions.set(RegionCatalogService.machineTypeOptions(region));
    }
  }

  onFormSubmit(data: { nodePools: NodePoolData[] }) {
    // Save node pools to state
    this.stateService.updateNodePools(data.nodePools);
    this.stateService.markStepCompleted(1);

    // Navigate to the next step
    this.router.navigate(['/clusters/add/summary']);
  }
}
