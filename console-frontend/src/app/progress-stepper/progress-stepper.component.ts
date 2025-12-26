import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';

export interface ProgressStep {
  name: string;
  route: string;
}

@Component({
  selector: 'app-progress-stepper',
  standalone: true,
  imports: [CommonModule, RouterLink],
  templateUrl: './progress-stepper.component.html',
  styleUrl: './progress-stepper.component.css',
})
export class ProgressStepperComponent {
  @Input() steps: ProgressStep[] = [];
  @Input() currentStepIndex = 0;

  isCompleted(index: number): boolean {
    return index < this.currentStepIndex;
  }

  isActive(index: number): boolean {
    return index === this.currentStepIndex;
  }
}
