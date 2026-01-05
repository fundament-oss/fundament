import { Component, ViewChild, ElementRef, AfterViewInit, inject, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router, ActivatedRoute } from '@angular/router';
import { TitleService } from '../title.service';
import { OrganizationApiService } from '../organization-api.service';
@Component({
  selector: 'app-add-cluster',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './add-cluster.component.html',
})
export class AddClusterComponent implements AfterViewInit, OnInit {
  @ViewChild('clusterNameInput') clusterNameInput!: ElementRef<HTMLInputElement>;

  private titleService = inject(TitleService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);
  private fb = inject(FormBuilder);
  private organizationApiService = inject(OrganizationApiService);

  // Form
  clusterForm: FormGroup;
  
  // Cluster ID from route (if editing existing cluster)
  clusterId: string | null = null;

  // Dropdown options based on Gardener
  regions = [
    { value: 'nl1', label: 'NL1' },
    { value: 'nl2', label: 'NL2' },
    { value: 'nl3', label: 'NL3' },
  ];

  kubernetesVersions = ['1.34.x', '1.28.x', '1.27.x', '1.26.x', '1.25.x'];

  constructor() {
    this.titleService.setTitle('Add cluster components');

    this.clusterForm = this.fb.group({
      clusterName: [
        '',
        [
          Validators.required,
          Validators.maxLength(253),
          Validators.pattern(/^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$/),
        ],
      ],
      region: ['nl1', Validators.required],
      kubernetesVersion: ['1.34.x', Validators.required],
    });
  }

  async ngOnInit() {
    // Get clusterId from route if it exists
    this.clusterId = this.route.snapshot.paramMap.get('clusterId');
    
    // If we have a clusterId, fetch the cluster data
    if (this.clusterId) {
      await this.loadClusterData(this.clusterId);
    }
  }

  ngAfterViewInit() {
    // Focus the cluster name input after the view is initialized
    this.clusterNameInput.nativeElement.focus();
  }

  private async loadClusterData(clusterId: string) {
    try {
      const cluster = await this.organizationApiService.getCluster(clusterId);
      this.clusterForm.patchValue({
        clusterName: cluster.name,
        region: cluster.region,
        kubernetesVersion: cluster.kubernetesVersion,
      });
    } catch (error) {
      console.error('Failed to load cluster data:', error);
      // TODO: Show error message to user
    }
  }

  get clusterName() {
    return this.clusterForm.get('clusterName');
  }

  getClusterNameError(): string {
    if (this.clusterName?.hasError('required')) {
      return 'The cluster name is required.';
    }
    if (this.clusterName?.hasError('maxlength')) {
      return 'The cluster name must not exceed 253 characters.';
    }
    if (this.clusterName?.hasError('pattern')) {
      return `The cluster name must contain only lowercase alphanumeric characters, '-' or '.', and start and end with an alphanumeric character.`;
    }
    return '';
  }

  async onSubmit() {
    if (this.clusterForm.invalid) {
      this.clusterForm.markAllAsTouched();
      this.scrollToFirstError();
      return;
    }

    const clusterData = this.clusterForm.value;
    
    try {
      const response = await this.organizationApiService.createCluster({
        name: clusterData.clusterName,
        region: clusterData.region,
        kubernetesVersion: clusterData.kubernetesVersion,
      });
      
      console.log('Cluster created successfully:', response);
      
      // Extract clusterId from response
      const clusterId = response.clusterId;
      
      // Navigate to the next step with clusterId
      this.router.navigate(['/add-cluster', clusterId, 'nodes']);
    } catch (error) {
      console.error('Failed to create cluster:', error);
      // TODO: Show error message to user
    }
  }

  private scrollToFirstError() {
    setTimeout(() => {
      const firstInvalidControl = document.querySelector('.ng-invalid:not(form)');
      if (firstInvalidControl) {
        firstInvalidControl.scrollIntoView({ behavior: 'smooth' });
        (firstInvalidControl as HTMLElement).focus();
      }
    }, 0);
  }

  onCancel() {
    this.router.navigate(['/']);
  }
}
