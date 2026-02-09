import {
  Component,
  inject,
  OnInit,
  ViewChild,
  ElementRef,
  AfterViewInit,
  signal,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { TitleService } from '../title.service';
import { AuthnApiService } from '../authn-api.service';

@Component({
  selector: 'app-login',
  imports: [CommonModule, ReactiveFormsModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './login.component.html',
})
export class LoginComponent implements OnInit, AfterViewInit {
  @ViewChild('emailInput') emailInput!: ElementRef<HTMLInputElement>;

  private titleService = inject(TitleService);

  private router = inject(Router);

  private apiService = inject(AuthnApiService);

  private fb = inject(FormBuilder);

  loginForm!: FormGroup;

  error = signal<string | null>(null);

  isLoading = signal(false);

  constructor() {
    this.titleService.setTitle('Log in');
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
      // User already authenticated, redirect to dashboard
      this.router.navigate(['/']);
    }
  }

  ngAfterViewInit() {
    // Focus the email input after the view is initialized
    this.emailInput.nativeElement.focus();
  }

  async onSubmit() {
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
      const returnUrl = localStorage.getItem('returnUrl') || '/';
      localStorage.removeItem('returnUrl');

      this.router.navigateByUrl(returnUrl);
    } catch (err) {
      this.error.set(err instanceof Error ? err.message : 'Login failed');
      this.isLoading.set(false);
    }
  }
}
