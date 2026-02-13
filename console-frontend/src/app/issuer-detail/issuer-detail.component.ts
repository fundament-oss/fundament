import { Component, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerPencil,
  tablerTrash,
  tablerAlertTriangle,
  tablerArrowLeft,
  tablerEye,
} from '@ng-icons/tabler-icons';
import ModalComponent from '../modal/modal.component';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { formatDate, formatDateTime } from '../utils/date-format';
import { type Issuer, MOCK_ISSUERS, MOCK_CERTIFICATES } from '../certificates/mock-data';

@Component({
  selector: 'app-issuer-detail',
  imports: [RouterLink, NgIcon, ModalComponent],
  viewProviders: [
    provideIcons({
      tablerPencil,
      tablerTrash,
      tablerAlertTriangle,
      tablerArrowLeft,
      tablerEye,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './issuer-detail.component.html',
})
export default class IssuerDetailComponent {
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private titleService = inject(TitleService);
  private toastService = inject(ToastService);

  issuer = signal<Issuer | null>(null);
  showDeleteModal = signal(false);

  // Certificates using this issuer
  relatedCertificates = signal<{ id: string; name: string; namespace: string; status: string }[]>(
    [],
  );

  constructor() {
    const id = this.route.snapshot.params['id'];
    const issuer = MOCK_ISSUERS.find((i) => i.id === id);
    if (issuer) {
      this.issuer.set(issuer);
      this.titleService.setTitle(issuer.name);

      // Find certificates referencing this issuer
      const related = MOCK_CERTIFICATES.filter((c) => c.issuerName === issuer.name).map((c) => ({
        id: c.id,
        name: c.name,
        namespace: c.namespace,
        status: c.status,
      }));
      this.relatedCertificates.set(related);
    } else {
      this.titleService.setTitle('Issuer not found');
    }
  }

  editIssuer() {
    this.toastService.info('Edit functionality coming soon');
  }

  confirmDelete() {
    this.showDeleteModal.set(false);
    this.toastService.success('Issuer deleted');
    this.router.navigate(['/issuers']);
  }

  formatDate(value: string | undefined): string {
    return formatDate(value, '—');
  }

  formatDateTime(value: string | undefined): string {
    return formatDateTime(value, '—');
  }
}
