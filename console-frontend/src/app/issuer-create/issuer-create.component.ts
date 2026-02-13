import { Component, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerTrash } from '@ng-icons/tabler-icons';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { type IssuerType, type IssuerKind, type SolverType } from '../certificates/mock-data';

interface SolverForm {
  type: SolverType;
  ingressClass: string;
  provider: string;
  selector: string;
}

@Component({
  selector: 'app-issuer-create',
  imports: [RouterLink, FormsModule, NgIcon],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerTrash,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './issuer-create.component.html',
})
export default class IssuerCreateComponent {
  private router = inject(Router);
  private titleService = inject(TitleService);
  private toastService = inject(ToastService);

  // Basic fields
  name = signal('');
  kind = signal<IssuerKind>('ClusterIssuer');
  namespace = signal('default');
  issuerType = signal<IssuerType>('ACME');

  // ACME fields
  acmeServer = signal('https://acme-v02.api.letsencrypt.org/directory');
  acmeEmail = signal('');
  acmePrivateKeySecret = signal('');
  solvers = signal<SolverForm[]>([{ type: 'HTTP01', ingressClass: 'nginx', provider: '', selector: '' }]);

  // CA fields
  caSecretName = signal('');

  namespaces = ['default', 'prod', 'staging', 'internal'];

  acmeServers = [
    { label: "Let's Encrypt Production", value: 'https://acme-v02.api.letsencrypt.org/directory' },
    {
      label: "Let's Encrypt Staging",
      value: 'https://acme-staging-v02.api.letsencrypt.org/directory',
    },
  ];

  constructor() {
    this.titleService.setTitle('Create issuer');
  }

  addSolver() {
    this.solvers.update((s) => [
      ...s,
      { type: 'HTTP01', ingressClass: '', provider: '', selector: '' },
    ]);
  }

  removeSolver(index: number) {
    this.solvers.update((s) => s.filter((_, i) => i !== index));
  }

  updateSolverType(index: number, type: SolverType) {
    this.solvers.update((s) =>
      s.map((solver, i) => (i === index ? { ...solver, type } : solver)),
    );
  }

  updateSolverIngressClass(index: number, ingressClass: string) {
    this.solvers.update((s) =>
      s.map((solver, i) => (i === index ? { ...solver, ingressClass } : solver)),
    );
  }

  updateSolverProvider(index: number, provider: string) {
    this.solvers.update((s) =>
      s.map((solver, i) => (i === index ? { ...solver, provider } : solver)),
    );
  }

  updateSolverSelector(index: number, selector: string) {
    this.solvers.update((s) =>
      s.map((solver, i) => (i === index ? { ...solver, selector } : solver)),
    );
  }

  create() {
    if (!this.name().trim()) {
      this.toastService.error('Please enter an issuer name');
      return;
    }

    if (this.issuerType() === 'ACME') {
      if (!this.acmeEmail().trim() || !this.acmePrivateKeySecret().trim()) {
        this.toastService.error('Please fill in all required ACME fields');
        return;
      }
    }

    if (this.issuerType() === 'CA') {
      if (!this.caSecretName().trim()) {
        this.toastService.error('Please enter a CA secret name');
        return;
      }
    }

    this.toastService.success(`Issuer "${this.name()}" created`);
    this.router.navigate(['/issuers']);
  }
}
