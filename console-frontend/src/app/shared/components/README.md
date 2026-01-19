# Shared Components Library

This directory contains reusable Angular standalone components for the console frontend application.

## Installation & Usage

All components are standalone and can be imported directly into your component:

```typescript
import { ButtonComponent, CardComponent } from '@app/shared/components';
```

Then add them to your component's imports array:

```typescript
@Component({
  selector: 'app-my-component',
  standalone: true,
  imports: [ButtonComponent, CardComponent],
  // ...
})
```

## Phase 1: Core UI Components

### Button Component

A versatile button component with multiple variants and sizes.

**Usage:**

```html
<!-- Primary button -->
<app-button variant="primary">Click me</app-button>

<!-- Secondary button with loading state -->
<app-button variant="secondary" [loading]="isLoading">Save</app-button>

<!-- Button with leading icon -->
<app-button variant="primary" [iconLeading]="true">
  <app-plus-icon slot="icon-leading" class="h-4 w-4" />
  Add Item
</app-button>

<!-- Router link button -->
<app-button variant="light" routerLink="/projects">Go to Projects</app-button>

<!-- Full width button -->
<app-button variant="primary" [block]="true">Full Width Button</app-button>
```

**Props:**

- `variant`: 'primary' | 'secondary' | 'light' | 'danger' | 'ghost' (default: 'primary')
- `size`: 'sm' | 'md' | 'lg' (default: 'md')
- `type`: 'button' | 'submit' | 'reset' (default: 'button')
- `disabled`: boolean (default: false)
- `loading`: boolean (default: false)
- `iconLeading`: boolean (default: false)
- `iconTrailing`: boolean (default: false)
- `href`: string (optional, for external links)
- `routerLink`: string | any[] (optional, for Angular routing)
- `block`: boolean (default: false)

**Events:**

- `(clicked)`: Emitted when button is clicked

---

### Card Component

A container component with optional header, footer, and loading state.

**Usage:**

```html
<!-- Basic card -->
<app-card>
  <p>Card content goes here</p>
</app-card>

<!-- Card with header -->
<app-card [hasHeader]="true">
  <h2 slot="header" class="text-xl font-bold">Card Title</h2>
  <p>Card content goes here</p>
</app-card>

<!-- Card with header actions -->
<app-card [hasHeader]="true" [hasHeaderActions]="true">
  <h2 slot="header" class="text-xl font-bold">Card Title</h2>
  <div slot="header-actions">
    <app-button variant="secondary" size="sm">Edit</app-button>
  </div>
  <p>Card content goes here</p>
</app-card>

<!-- Card with footer -->
<app-card [hasFooter]="true">
  <p>Card content goes here</p>
  <div slot="footer" class="flex justify-end gap-2">
    <app-button variant="secondary">Cancel</app-button>
    <app-button variant="primary">Save</app-button>
  </div>
</app-card>

<!-- Loading card -->
<app-card [loading]="true"></app-card>
```

**Props:**

- `hasHeader`: boolean (default: false)
- `hasHeaderActions`: boolean (default: false)
- `hasFooter`: boolean (default: false)
- `padding`: boolean (default: true)
- `outlined`: boolean (default: false)
- `elevated`: boolean (default: false)
- `loading`: boolean (default: false)

---

### Table Component

A flexible table component with sorting, loading, and empty states.

**Usage:**

```typescript
// In your component
interface User {
  id: string;
  name: string;
  email: string;
  role: string;
}

columns: TableColumn<User>[] = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'email', label: 'Email', sortable: true },
  { key: 'role', label: 'Role', align: 'center' },
];

data: User[] = [...];

handleRowClick(user: User) {
  console.log('Clicked user:', user);
}

handleSort(event: TableSortEvent) {
  console.log('Sort:', event);
}
```

```html
<app-table
  [columns]="columns"
  [data]="data"
  [loading]="isLoading"
  [clickable]="true"
  [hasActions]="true"
  emptyMessage="No users found"
  (rowClick)="handleRowClick($event)"
  (sort)="handleSort($event)"
>
  <ng-template slot="actions" let-row>
    <button (click)="editUser(row)">Edit</button>
    <button (click)="deleteUser(row)">Delete</button>
  </ng-template>
</app-table>
```

**Props:**

- `columns`: TableColumn[] (required)
- `data`: T[] (required)
- `loading`: boolean (default: false)
- `hoverable`: boolean (default: true)
- `clickable`: boolean (default: false)
- `showHeader`: boolean (default: true)
- `hasActions`: boolean (default: false)
- `emptyMessage`: string (default: 'No data available')
- `sortColumn`: string | null (default: null)
- `sortDirection`: 'asc' | 'desc' | null (default: null)
- `trackBy`: function (optional)

**Events:**

- `(rowClick)`: Emitted when a row is clicked (if clickable is true)
- `(sort)`: Emitted when a column header is clicked (if sortable)

---

## Phase 2: Forms & Navigation Components

### Form Input Component

A form input with label, help text, and error message support.

**Usage:**

```html
<app-form-input
  id="username"
  label="Username"
  type="text"
  placeholder="Enter your username"
  helpText="Choose a unique username"
  [error]="usernameError"
  [required]="true"
  [(ngModel)]="username"
></app-form-input>
```

**Props:**

- `id`: string (required)
- `label`: string
- `type`: 'text' | 'email' | 'password' | 'number' | 'date' (default: 'text')
- `placeholder`: string
- `helpText`: string
- `error`: string
- `required`: boolean (default: false)
- `disabled`: boolean (default: false)

---

### Form Select Component

A select dropdown with label and error support.

**Usage:**

```typescript
options: SelectOption[] = [
  { label: 'Option 1', value: 'opt1' },
  { label: 'Option 2', value: 'opt2' },
  { label: 'Disabled', value: 'opt3', disabled: true },
];
```

```html
<app-form-select
  id="region"
  label="Region"
  [options]="options"
  placeholder="Select a region"
  [error]="regionError"
  [(ngModel)]="selectedRegion"
></app-form-select>
```

**Props:**

- `id`: string (required)
- `label`: string
- `options`: SelectOption[] (required)
- `placeholder`: string
- `helpText`: string
- `error`: string
- `required`: boolean (default: false)
- `disabled`: boolean (default: false)
- `fullWidth`: boolean (default: true)

---

### Form Textarea Component

A textarea with label and error support.

**Usage:**

```html
<app-form-textarea
  id="description"
  label="Description"
  placeholder="Enter a description"
  [rows]="5"
  [error]="descriptionError"
  [(ngModel)]="description"
></app-form-textarea>
```

**Props:**

- `id`: string (required)
- `label`: string
- `placeholder`: string
- `helpText`: string
- `error`: string
- `required`: boolean (default: false)
- `disabled`: boolean (default: false)
- `rows`: number (default: 3)

---

### Tabs Component

A tabs component with optional router integration.

**Usage:**

```typescript
tabs: Tab[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'settings', label: 'Settings', badge: 3 },
  { id: 'disabled', label: 'Disabled', disabled: true },
];

// With router links
tabsWithRouter: Tab[] = [
  { id: 'nodes', label: 'Nodes', routerLink: '/clusters/123/nodes' },
  { id: 'plugins', label: 'Plugins', routerLink: '/clusters/123/plugins' },
];
```

```html
<!-- Manual tab switching -->
<app-tabs [tabs]="tabs" [activeTab]="currentTab" (tabChange)="onTabChange($event)">
  <div>Tab content for {{ currentTab }}</div>
</app-tabs>

<!-- Router-integrated tabs -->
<app-tabs [tabs]="tabsWithRouter" [useRouter]="true"></app-tabs>
```

**Props:**

- `tabs`: Tab[] (required)
- `activeTab`: string
- `orientation`: 'horizontal' | 'vertical' (default: 'horizontal')
- `useRouter`: boolean (default: false)

**Events:**

- `(tabChange)`: Emitted when a tab is clicked

---

### Modal Component

A modal dialog with customizable size and behavior.

**Usage:**

```html
<app-modal
  [isOpen]="showModal"
  size="md"
  [hasHeader]="true"
  [hasFooter]="true"
  [closeOnBackdrop]="true"
  (closed)="onModalClose()"
>
  <h2 slot="header">Modal Title</h2>

  <p>Modal content goes here</p>

  <div slot="footer" class="flex justify-end gap-3">
    <app-button variant="secondary" (clicked)="showModal = false">Cancel</app-button>
    <app-button variant="primary" (clicked)="handleSave()">Save</app-button>
  </div>
</app-modal>
```

**Props:**

- `isOpen`: boolean (default: false)
- `size`: 'sm' | 'md' | 'lg' | 'xl' | 'full' (default: 'md')
- `hasHeader`: boolean (default: true)
- `hasFooter`: boolean (default: false)
- `showCloseButton`: boolean (default: true)
- `closeOnBackdrop`: boolean (default: true)
- `closeOnEscape`: boolean (default: true)
- `scrollable`: boolean (default: true)
- `titleId`: string (default: 'modal-title')

**Events:**

- `(closed)`: Emitted when the modal is closed

---

## Phase 3: Polish Components

### Badge Component

A badge component for labels, status indicators, and counts.

**Usage:**

```html
<app-badge variant="default">Default</app-badge>
<app-badge variant="success">Success</app-badge>
<app-badge variant="warning">Warning</app-badge>
<app-badge variant="danger">Danger</app-badge>
<app-badge variant="info" [dot]="true">With Dot</app-badge>
<app-badge variant="purple" size="lg">Large</app-badge>
```

**Props:**

- `variant`: 'default' | 'success' | 'warning' | 'danger' | 'info' | 'purple' | 'blue' | 'green' (default: 'default')
- `size`: 'sm' | 'md' | 'lg' (default: 'md')
- `dot`: boolean (default: false)
- `ariaLabel`: string

---

### Spinner Component

A loading spinner with multiple variants and sizes.

**Usage:**

```html
<!-- Border spinner (default) -->
<app-spinner size="md"></app-spinner>

<!-- Dots variant -->
<app-spinner variant="dots" size="lg"></app-spinner>

<!-- Custom color -->
<app-spinner [color]="'#6366f1'"></app-spinner>

<!-- Different sizes -->
<app-spinner size="sm"></app-spinner>
<app-spinner size="md"></app-spinner>
<app-spinner size="lg"></app-spinner>
<app-spinner size="xl"></app-spinner>
```

**Props:**

- `size`: 'sm' | 'md' | 'lg' | 'xl' (default: 'md')
- `variant`: 'border' | 'dots' (default: 'border')
- `color`: string (optional)
- `ariaLabel`: string (default: 'Loading')

---

### Empty State Component

A component for displaying empty states with optional actions.

**Usage:**

```html
<app-empty-state
  title="No projects found"
  description="Create your first project to get started"
  [hasIcon]="true"
  [hasPrimaryAction]="true"
>
  <svg
    slot="icon"
    class="h-6 w-6 text-gray-400"
    fill="none"
    stroke="currentColor"
    viewBox="0 0 24 24"
  >
    <path
      stroke-linecap="round"
      stroke-linejoin="round"
      stroke-width="2"
      d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
    />
  </svg>

  <app-button slot="primary-action" variant="primary" routerLink="/projects/add">
    Create Project
  </app-button>
</app-empty-state>
```

**Props:**

- `title`: string
- `description`: string
- `hasIcon`: boolean (default: false)
- `hasPrimaryAction`: boolean (default: false)
- `hasSecondaryAction`: boolean (default: false)
- `padding`: boolean (default: true)

---

### Alert Component

An alert component for displaying important messages.

**Usage:**

```html
<!-- Basic alerts -->
<app-alert variant="success">Operation completed successfully!</app-alert>
<app-alert variant="warning">Please review the changes before saving.</app-alert>
<app-alert variant="danger">An error occurred. Please try again.</app-alert>
<app-alert variant="info">New features are now available.</app-alert>

<!-- Alert with title and icon -->
<app-alert
  variant="danger"
  title="Error"
  [hasIcon]="true"
  [dismissible]="true"
  (dismissed)="onAlertDismissed()"
>
  <app-error-icon slot="icon" class="h-5 w-5" />
  Failed to save changes. Please check your connection and try again.
</app-alert>

<!-- Alert with actions -->
<app-alert variant="info" title="Update Available" [hasActions]="true">
  A new version of the application is available.

  <div slot="actions" class="flex gap-2">
    <app-button variant="light" size="sm">Update Now</app-button>
    <app-button variant="ghost" size="sm">Remind Me Later</app-button>
  </div>
</app-alert>
```

**Props:**

- `variant`: 'success' | 'warning' | 'danger' | 'info' (default: 'info')
- `title`: string
- `dismissible`: boolean (default: false)
- `hasIcon`: boolean (default: false)
- `hasActions`: boolean (default: false)

**Events:**

- `(dismissed)`: Emitted when the alert is dismissed

---

### Progress Bar Component

A progress bar for showing completion status.

**Usage:**

```html
<!-- Basic progress bar -->
<app-progress-bar [value]="75"></app-progress-bar>

<!-- With label -->
<app-progress-bar
  [value]="uploadProgress"
  label="Upload Progress"
  [showLabel]="true"
  variant="success"
></app-progress-bar>

<!-- Different variants -->
<app-progress-bar [value]="50" variant="default"></app-progress-bar>
<app-progress-bar [value]="75" variant="success"></app-progress-bar>
<app-progress-bar [value]="85" variant="warning"></app-progress-bar>
<app-progress-bar [value]="95" variant="danger"></app-progress-bar>

<!-- Different sizes -->
<app-progress-bar [value]="50" size="sm"></app-progress-bar>
<app-progress-bar [value]="50" size="md"></app-progress-bar>
<app-progress-bar [value]="50" size="lg"></app-progress-bar>
```

**Props:**

- `value`: number (default: 0)
- `max`: number (default: 100)
- `variant`: 'default' | 'success' | 'warning' | 'danger' (default: 'default')
- `size`: 'sm' | 'md' | 'lg' (default: 'md')
- `label`: string
- `showLabel`: boolean (default: false)
- `showPercentage`: boolean (default: true)

---

### Breadcrumbs Component

A breadcrumb navigation component.

**Usage:**

```typescript
breadcrumbs: Breadcrumb[] = [
  { label: 'Home', url: '/' },
  { label: 'Projects', url: '/projects' },
  { label: 'Project Details' },
];
```

```html
<app-breadcrumbs [breadcrumbs]="breadcrumbs"></app-breadcrumbs>
```

**Props:**

- `breadcrumbs`: Breadcrumb[] (required)

---

### Skeleton Component

A skeleton loader for content placeholders.

**Usage:**

```html
<!-- Text skeleton -->
<app-skeleton variant="text" width="200px" height="1rem"></app-skeleton>

<!-- Circular skeleton (for avatars) -->
<app-skeleton variant="circular" width="48px" height="48px"></app-skeleton>

<!-- Rectangular skeleton -->
<app-skeleton variant="rectangular" width="100%" height="200px"></app-skeleton>

<!-- Multiple skeletons for a card layout -->
<div class="space-y-3">
  <app-skeleton variant="text" width="60%" height="1.5rem"></app-skeleton>
  <app-skeleton variant="text" width="100%" height="1rem"></app-skeleton>
  <app-skeleton variant="text" width="100%" height="1rem"></app-skeleton>
  <app-skeleton variant="text" width="80%" height="1rem"></app-skeleton>
</div>
```

**Props:**

- `variant`: 'text' | 'circular' | 'rectangular' (default: 'text')
- `width`: string (default: '100%')
- `height`: string (default: '1rem')

---

## Design Principles

1. **Standalone Components**: All components are standalone and can be imported individually
2. **Tailwind CSS**: Uses Tailwind utility classes for styling
3. **Dark Mode Support**: All components support dark mode via Tailwind's dark variant
4. **Accessibility**: Components include proper ARIA labels and keyboard navigation where applicable
5. **TypeScript**: Fully typed with interfaces for all component inputs and outputs
6. **Minimal Theming**: Only variant-based theming (no complex theme systems)

## Migration Guide

To migrate existing code to use these components:

1. Import the component: `import { ButtonComponent } from '@app/shared/components';`
2. Add to imports array in your component
3. Replace existing HTML with component usage
4. Update any custom CSS classes to use component props instead

Example:

```html
<!-- Before -->
<button class="btn-primary">Click me</button>

<!-- After -->
<app-button variant="primary">Click me</app-button>
```
