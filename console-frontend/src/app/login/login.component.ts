import '@nldd/design-system/top-navigation-bar';
import {
  Component,
  inject,
  OnInit,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import AutofocusDirective from '../autofocus.directive';
import { TitleService } from '../title.service';
import AuthnApiService from '../authn-api.service';
import '@nldd/design-system/password-field';

/**
 * Where logging in lands you when nothing else was asked for: the clusters,
 * which is what the console is mostly read for. An address you were sent to
 * before the login page wins over it, and the organization is filled in on the
 * way there.
 */
const DEFAULT_ROUTE = '/clusters';

@Component({
  selector: 'app-login',
  imports: [ReactiveFormsModule, AutofocusDirective],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './login.component.html',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  // The host is a flex item of nldd-app-view, so it needs a display and a share
  // of the height, or the page inside it collapses to its content.
  styles: ':host { display: block; flex: 1; min-height: 0; }',
})
export default class LoginComponent implements OnInit {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private apiService = inject(AuthnApiService);

  private fb = inject(FormBuilder);

  loginForm!: FormGroup;

  /** Whether the form has been submitted at least once. Before that, a field
   * that is not finished yet is not a mistake, only unfinished. */
  formSubmitted = signal(false);

  error = signal<string | null>(null);

  isLoading = signal(false);

  constructor() {
    this.titleService.setTitle('Log in');
    this.titleService.setDescription(
      'Log in - Fundament: Open-source platform for deploying and managing Kubernetes clusters with bare-metal provisioning',
    );
    this.loginForm = this.fb.group({
      email: ['', [Validators.required, Validators.email]],
      password: ['', [Validators.required]],
    });
  }

  get email() {
    return this.loginForm.get('email');
  }

  get password() {
    return this.loginForm.get('password');
  }

  getEmailError(): string {
    if (this.email?.hasError('required')) {
      return 'Email address is required';
    }
    if (this.email?.hasError('email')) {
      return 'Please enter a valid email address';
    }
    return '';
  }

  getPasswordError(): string {
    if (this.password?.hasError('required')) {
      return 'Password is required';
    }
    return '';
  }

  async ngOnInit() {
    // Check if user is already authenticated (check state first to avoid unnecessary API call)
    if (this.apiService.isAuthenticated()) {
      // User already authenticated, redirect to the default page
      this.router.navigateByUrl(DEFAULT_ROUTE);
    }
  }

  async onSubmit(event?: Event) {
    // Prevent the native form submission triggered by the submit button.
    event?.preventDefault();
    this.formSubmitted.set(true);

    if (this.isLoading()) return;
    if (this.loginForm.invalid) {
      this.loginForm.markAllAsTouched();
      return;
    }

    this.isLoading.set(true);
    this.error.set(null);

    try {
      const { email, password } = this.loginForm.value;
      await this.apiService.login(email, password);
      // Login successful, redirect to the return URL or dashboard
      const returnUrl = localStorage.getItem('returnUrl') || DEFAULT_ROUTE;
      localStorage.removeItem('returnUrl');

      this.router.navigateByUrl(returnUrl);
    } catch (err) {
      this.error.set(err instanceof Error ? `Login failed: ${err.message}` : 'Login failed');
      this.isLoading.set(false);
    }
  }
}
