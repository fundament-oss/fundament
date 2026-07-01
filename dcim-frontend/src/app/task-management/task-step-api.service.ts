import { Injectable, inject } from '@angular/core';
import type { TaskStep as ProtoTaskStep } from '../../generated/v1/task_pb';
import { TASK_STEP_CLIENT } from '../../connect/tokens';

/** A task checklist step (presentation icon/svg are added by the component). */
export interface TaskStepData {
  id: string;
  taskId: string;
  title: string;
  description: string;
  ordinal: number;
  completed: boolean;
}

@Injectable({ providedIn: 'root' })
export default class TaskStepApiService {
  private readonly client = inject(TASK_STEP_CLIENT);

  static mapStep(s: ProtoTaskStep): TaskStepData {
    return {
      id: s.id,
      taskId: s.taskId,
      title: s.title,
      description: s.description,
      ordinal: s.ordinal,
      completed: s.completed,
    };
  }

  listTaskSteps(taskId: string) {
    return this.client.listTaskSteps({ taskId });
  }

  updateTaskStep(id: string, completed: boolean) {
    return this.client.updateTaskStep({ id, completed });
  }
}
