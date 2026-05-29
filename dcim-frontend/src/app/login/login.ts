import {
  Component,
  inject,
  OnInit,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import AuthService from '../auth.service';

@Component({
  selector: 'app-login',
  imports: [ReactiveFormsModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    <div class="flex min-h-screen flex-col justify-center py-12 sm:px-6 lg:px-8">
      <div class="sm:mx-auto sm:w-full sm:max-w-md">
        <h2 class="mb-6 text-center text-3xl font-bold">Log in to DCIM</h2>
        <p class="mb-8 text-center text-gray-600 dark:text-gray-300">
          Please enter your credentials to continue.
        </p>
      </div>
      <div class="sm:mx-auto sm:w-full sm:max-w-md">
        <div
          class="rounded-lg border border-gray-200 bg-white px-4 py-8 sm:px-10 dark:border-gray-800 dark:bg-gray-950"
        >
          <form [formGroup]="loginForm" class="space-y-6" (ngSubmit)="onSubmit()">
            <nldd-form-field label="Email address">
              <nldd-text-field
                type="email"
                id="email"
                placeholder="Enter your email address"
                autocomplete="email"
                [value]="email?.value ?? ''"
                (input)="loginForm.get('email')!.setValue($any($event.target).value)"
                (blur)="loginForm.get('email')!.markAsTouched()"
                [invalid]="!!(email?.invalid && email?.touched)"
                error-message="login-email-error"
              ></nldd-text-field>
              <nldd-form-field-error-text id="login-email-error">
                {{ getEmailError() }}
              </nldd-form-field-error-text>
            </nldd-form-field>

            <nldd-form-field label="Password">
              <nldd-password-field
                id="password"
                placeholder="Enter your password"
                autocomplete="current-password"
                [value]="password?.value ?? ''"
                (input)="loginForm.get('password')!.setValue($any($event.target).value)"
                (blur)="loginForm.get('password')!.markAsTouched()"
                show-text="Show"
                show-accessible-label="Show password"
                hide-text="Hide"
                hide-accessible-label="Hide password"
                [invalid]="!!(password?.invalid && password?.dirty)"
                error-message="login-password-error"
              ></nldd-password-field>
              <nldd-form-field-error-text id="login-password-error">
                {{ getPasswordError() }}
              </nldd-form-field-error-text>
            </nldd-form-field>

            @if (error()) {
              <div class="rounded-md bg-red-50 p-3 dark:bg-red-900/20">
                <p class="text-red-800 dark:text-red-200">{{ error() }}</p>
              </div>
            }

            <nldd-button
              variant="primary"
              full-width
              [attr.disabled]="isLoading() ? '' : null"
              [text]="isLoading() ? 'Signing in...' : 'Sign in'"
              type="submit"
            ></nldd-button>
          </form>
        </div>
      </div>
    </div>
  `,
})
export default class LoginComponent implements OnInit {
  private router = inject(Router);

  private authService = inject(AuthService);

  private fb = inject(FormBuilder);

  loginForm = this.fb.group({
    email: ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required]],
  });

  error = signal<string | null>(null);

  isLoading = signal(false);

  get email() {
    return this.loginForm.get('email');
  }

  get password() {
    return this.loginForm.get('password');
  }

  getEmailError(): string {
    if (this.email?.hasError('required')) return 'Email address is required';
    if (this.email?.hasError('email')) return 'Please enter a valid email address';
    return '';
  }

  getPasswordError(): string {
    if (this.password?.hasError('required')) return 'Password is required';
    return '';
  }

  async ngOnInit() {
    if (this.authService.isAuthenticated()) {
      this.router.navigate(['/']);
    }
  }

  async onSubmit() {
    if (this.isLoading()) return;
    if (this.loginForm.invalid) {
      this.loginForm.markAllAsTouched();
      return;
    }

    this.isLoading.set(true);
    this.error.set(null);

    try {
      const { email, password } = this.loginForm.value;
      await this.authService.login(email!, password!);
      const returnUrl = localStorage.getItem('returnUrl') ?? '/';
      localStorage.removeItem('returnUrl');
      this.router.navigateByUrl(returnUrl);
    } catch (err) {
      this.error.set(err instanceof Error ? `Login failed: ${err.message}` : 'Login failed');
      this.isLoading.set(false);
    }
  }
}
