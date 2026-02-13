import { Component, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPlus,
  tablerEye,
  tablerPencil,
  tablerTrash,
  tablerAlertTriangle,
  tablerShieldCheck,
  tablerArrowRight,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../modal/modal.component';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { formatDate } from '../utils/date-format';
import { type Certificate, type CertificateStatus, MOCK_CERTIFICATES } from './mock-data';

@Component({
  selector: 'app-certificates',
  imports: [RouterLink, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerEye,
      tablerPencil,
      tablerTrash,
      tablerAlertTriangle,
      tablerShieldCheck,
      tablerArrowRight,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './certificates.component.html',
})
export default class CertificatesComponent {
  private titleService = inject(TitleService);
  private toastService = inject(ToastService);

  certificates = signal<Certificate[]>([...MOCK_CERTIFICATES]);

  showDeleteModal = signal(false);
  pendingCertId = signal<string | null>(null);
  pendingCertName = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Certificates');
  }

  getStatusBadgeClass(status: CertificateStatus): string {
    switch (status) {
      case 'Ready':
        return 'badge badge-emerald';
      case 'NotReady':
        return 'badge badge-yellow';
      case 'Expired':
        return 'badge badge-rose';
    }
  }

  getStatusLabel(status: CertificateStatus): string {
    switch (status) {
      case 'Ready':
        return 'Ready';
      case 'NotReady':
        return 'Not Ready';
      case 'Expired':
        return 'Expired';
    }
  }

  formatDnsNames(dnsNames: string[]): string {
    return dnsNames.join(', ');
  }

  editCertificate() {
    this.toastService.info('Edit functionality coming soon');
  }

  openDeleteModal(id: string, name: string) {
    this.pendingCertId.set(id);
    this.pendingCertName.set(name);
    this.showDeleteModal.set(true);
  }

  confirmDelete() {
    const id = this.pendingCertId();
    if (!id) return;

    this.certificates.update((certs) => certs.filter((c) => c.id !== id));
    this.showDeleteModal.set(false);
    this.toastService.success('Certificate deleted');
  }

  formatDate(value: string | undefined): string {
    return formatDate(value, '—');
  }
}
