import { Component, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPencil,
  tablerTrash,
  tablerAlertTriangle,
  tablerArrowLeft,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../modal/modal.component';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { formatDate, formatDateTime } from '../utils/date-format';
import {
  type Certificate,
  type CertificateEvent,
  MOCK_CERTIFICATES,
} from '../certificates/mock-data';

@Component({
  selector: 'app-certificate-detail',
  imports: [RouterLink, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPencil,
      tablerTrash,
      tablerAlertTriangle,
      tablerArrowLeft,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './certificate-detail.component.html',
})
export default class CertificateDetailComponent {
  private route = inject(ActivatedRoute);

  private router = inject(Router);

  private titleService = inject(TitleService);

  private toastService = inject(ToastService);

  certificate = signal<Certificate | null>(null);

  showDeleteModal = signal(false);

  constructor() {
    const id = this.route.snapshot.params['id'];
    const cert = MOCK_CERTIFICATES.find((c) => c.id === id);
    if (cert) {
      this.certificate.set(cert);
      this.titleService.setTitle(cert.name);
    } else {
      this.titleService.setTitle('Certificate not found');
    }
  }

  editCertificate() {
    this.toastService.info('Edit functionality coming soon');
  }

  confirmDelete() {
    this.showDeleteModal.set(false);
    this.toastService.success('Certificate deleted');
    this.router.navigate(['/certificates']);
  }

  // eslint-disable-next-line class-methods-use-this
  getEventDotColor(type: CertificateEvent['type']): string {
    switch (type) {
      case 'Issued':
        return 'bg-emerald-500';
      case 'Requested':
        return 'bg-blue-500';
      case 'Created':
        return 'bg-gray-400';
      case 'Renewal':
        return 'bg-indigo-500';
      case 'Failed':
        return 'bg-rose-500';
      default: {
        const exhaustive: never = type;
        throw new Error(`Unhandled event type: ${exhaustive}`);
      }
    }
  }

  // eslint-disable-next-line class-methods-use-this
  formatDate(value: string | undefined): string {
    return formatDate(value, '—');
  }

  // eslint-disable-next-line class-methods-use-this
  formatDateTime(value: string | undefined): string {
    return formatDateTime(value, '—');
  }
}
