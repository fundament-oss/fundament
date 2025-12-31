import {
  Component,
  inject,
  OnInit,
  ViewChild,
  ElementRef,
  AfterViewInit,
  signal,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { Title } from '@angular/platform-browser';
import { Router } from '@angular/router';
import { ApiService } from '../api.service';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './login.component.html',
})
export class LoginComponent implements OnInit, AfterViewInit {
  @ViewChild('emailInput') emailInput!: ElementRef<HTMLInputElement>;

  private titleService = inject(Title);
  private router = inject(Router);
  private apiService = inject(ApiService);
  private fb = inject(FormBuilder);

  loginForm!: FormGroup;
  error = signal<string | null>(null);
  isLoading = signal(false);

  constructor() {
    this.titleService.setTitle('Inloggen — Fundament Console');
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
      return;
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
