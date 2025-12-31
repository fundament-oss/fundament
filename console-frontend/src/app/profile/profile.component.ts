import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { RouterLink, Router } from '@angular/router';
import { ApiService, UserInfo } from '../api.service';
import { Title } from '@angular/platform-browser';

@Component({
  selector: 'app-profile',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, RouterLink],
  templateUrl: './profile.component.html',
})
export class ProfileComponent implements OnInit {
  private titleService = inject(Title);
  private fb = inject(FormBuilder);
  private apiService = inject(ApiService);
  private router = inject(Router);

  profileForm: FormGroup;
  userInfo = signal<UserInfo | null>(null);
  isLoading = signal(true);
  error = signal<string | null>(null);

  constructor() {
    this.titleService.setTitle('Profile — Fundament Console');

    this.profileForm = this.fb.group({
      fullName: ['', Validators.required],
      currentPassword: ['', Validators.required],
      password: ['', [Validators.minLength(8)]],
    });
  }

  async ngOnInit() {
    await this.loadUserInfo();
  }

  private async loadUserInfo() {
    try {
      const user = await this.apiService.getUserInfo();
      this.userInfo.set(user);
      this.profileForm.patchValue({
        fullName: user.name || '',
      });
      this.isLoading.set(false);
    } catch (error) {
      console.error('Failed to load user info:', error);
      this.error.set('Failed to load user information');
      this.isLoading.set(false);
      // Redirect to login if not authenticated
      this.router.navigate(['/login']);
    }
  }

  get fullName() {
    return this.profileForm.get('fullName');
  }

  get currentPassword() {
    return this.profileForm.get('currentPassword');
  }

  get password() {
    return this.profileForm.get('password');
  }

  getFullNameError(): string {
    if (this.fullName?.hasError('required')) {
      return 'Full name is required.';
    }
    return '';
  }

  getCurrentPasswordError(): string {
    if (this.currentPassword?.hasError('required')) {
      return 'Current password is required to make changes.';
    }
    return '';
  }

  getPasswordError(): string {
    if (this.password?.hasError('minlength')) {
      return 'Password must be at least 8 characters.';
    }
    return '';
  }

  onSave(): void {
    if (this.profileForm.invalid) {
      this.profileForm.markAllAsTouched();
      this.scrollToFirstError();
      return;
    }

    // Save logic would go here
    console.log('Saving profile:', this.profileForm.value);
  }

  private scrollToFirstError() {
    setTimeout(() => {
      const firstInvalidControl = document.querySelector('.ng-invalid:not(form)');
      if (firstInvalidControl) {
        firstInvalidControl.scrollIntoView({ behavior: 'smooth' });
        (firstInvalidControl as HTMLElement).focus();
      }
    }, 0);
  }
}
