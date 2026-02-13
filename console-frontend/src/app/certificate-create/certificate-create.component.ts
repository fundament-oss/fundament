import { Component, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { NgIcon, provideIcons } from '@ng-icons/core';
import { tablerPlus, tablerX } from '@ng-icons/tabler-icons';
import { TitleService } from '../title.service';
import { ToastService } from '../toast.service';
import { MOCK_ISSUERS } from '../certificates/mock-data';

@Component({
  selector: 'app-certificate-create',
  imports: [RouterLink, FormsModule, NgIcon],
  viewProviders: [
    provideIcons({
      tablerPlus,
      tablerX,
    }),
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './certificate-create.component.html',
})
export default class CertificateCreateComponent {
  private router = inject(Router);
  private titleService = inject(TitleService);
  private toastService = inject(ToastService);

  // Form fields
  name = signal('');
  namespace = signal('default');
  secretName = signal('');
  issuerKind = signal<'ClusterIssuer' | 'Issuer'>('ClusterIssuer');
  issuerName = signal('');
  duration = signal('2160h');
  renewBefore = signal('720h');
  algorithm = signal('RSA');
  keySize = signal('2048');
  encoding = signal('PKCS1');
  rotationPolicy = signal('Never');

  // DNS names
  dnsNameInput = signal('');
  dnsNames = signal<string[]>([]);

  // IP addresses
  ipAddressInput = signal('');
  ipAddresses = signal<string[]>([]);

  // Available issuers filtered by kind
  availableIssuers = MOCK_ISSUERS;

  namespaces = ['default', 'prod', 'staging', 'internal'];

  constructor() {
    this.titleService.setTitle('Create certificate');
  }

  get filteredIssuers() {
    return this.availableIssuers.filter((i) => i.kind === this.issuerKind());
  }

  addDnsName() {
    const value = this.dnsNameInput().trim();
    if (value && !this.dnsNames().includes(value)) {
      this.dnsNames.update((names) => [...names, value]);
      this.dnsNameInput.set('');
    }
  }

  removeDnsName(name: string) {
    this.dnsNames.update((names) => names.filter((n) => n !== name));
  }

  addIpAddress() {
    const value = this.ipAddressInput().trim();
    if (value && !this.ipAddresses().includes(value)) {
      this.ipAddresses.update((ips) => [...ips, value]);
      this.ipAddressInput.set('');
    }
  }

  removeIpAddress(ip: string) {
    this.ipAddresses.update((ips) => ips.filter((i) => i !== ip));
  }

  onDnsKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      this.addDnsName();
    }
  }

  onIpKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      this.addIpAddress();
    }
  }

  create() {
    if (!this.name().trim() || !this.secretName().trim() || !this.issuerName().trim()) {
      this.toastService.error('Please fill in all required fields');
      return;
    }

    if (this.dnsNames().length === 0 && this.ipAddresses().length === 0) {
      this.toastService.error('Please add at least one DNS name or IP address');
      return;
    }

    this.toastService.success(`Certificate "${this.name()}" created`);
    this.router.navigate(['/certificates']);
  }
}
