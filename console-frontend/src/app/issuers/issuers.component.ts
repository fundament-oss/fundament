import { Component, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerEye,
  tablerPencil,
  tablerTrash,
  tablerAlertTriangle,
  tablerCertificate,
  tablerArrowRight,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../modal/modal.component';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { type Issuer, MOCK_ISSUERS } from '../certificates/mock-data';

@Component({
  selector: 'app-issuers',
  imports: [RouterLink, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerEye,
      tablerPencil,
      tablerTrash,
      tablerAlertTriangle,
      tablerCertificate,
      tablerArrowRight,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './issuers.component.html',
})
export default class IssuersComponent {
  private titleService = inject(TitleService);
  private toastService = inject(ToastService);

  issuers = signal<Issuer[]>([...MOCK_ISSUERS]);

  showDeleteModal = signal(false);
  pendingIssuerId = signal<string | null>(null);
  pendingIssuerName = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Issuers');
  }

  getKindBadgeClass(kind: string): string {
    return kind === 'ClusterIssuer' ? 'badge badge-purple' : 'badge badge-blue';
  }

  editIssuer() {
    this.toastService.info('Edit functionality coming soon');
  }

  openDeleteModal(id: string, name: string) {
    this.pendingIssuerId.set(id);
    this.pendingIssuerName.set(name);
    this.showDeleteModal.set(true);
  }

  confirmDelete() {
    const id = this.pendingIssuerId();
    if (!id) return;

    this.issuers.update((list) => list.filter((i) => i.id !== id));
    this.showDeleteModal.set(false);
    this.toastService.success('Issuer deleted');
  }
}
