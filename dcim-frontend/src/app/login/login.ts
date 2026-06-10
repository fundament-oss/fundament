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
import ThemeToggleComponent from '../shared/theme-toggle';
import AutofocusDirective from '../autofocus.directive';

@Component({
  selector: 'app-login',
  imports: [ReactiveFormsModule, ThemeToggleComponent, AutofocusDirective],
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './login.html',
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
