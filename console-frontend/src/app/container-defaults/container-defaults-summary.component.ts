import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
} from '@angular/core';

import MockBadgeComponent from '../mock-badge/mock-badge.component';
import type { DefaultsSummarySection } from './container-defaults';

import '@nldd/design-system/icon-cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';

/**
 * What is stored, not a form: you come to a detail page to look, and a page
 * full of controls invites changes nobody meant to make. Every row opens the
 * same editor, and the icon says so.
 *
 * Shared by the cluster and the project block: the rows differ only in what
 * their values mean, which the caller has already worked out.
 */
@Component({
  selector: 'app-container-defaults-summary',
  imports: [MockBadgeComponent],
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    @for (section of sections(); track section.copy.name) {
      <nldd-title size="5">
        <h4>
          {{ section.copy.title }}&ngsp;
          <app-mock-badge
            label="Mock: container defaults are not final yet"
            explanation="Container defaults are saved and applied to your namespaces, but this part of the console is still being worked out. How it works may change."
          ></app-mock-badge>
        </h4>
      </nldd-title>
      <nldd-spacer size="12"></nldd-spacer>
      <nldd-list
        variant="box-tinted"
        [attr.accessible-label]="section.copy.title"
      >
        @for (row of section.rows; track row.label) {
          <nldd-list-item
            button
            (click)="edit.emit()"
          >
            <nldd-text-cell
              width="full"
              [attr.text]="row.label"
            ></nldd-text-cell>
            <nldd-spacer-cell size="12"></nldd-spacer-cell>
            <nldd-text-cell
              width="fit-content"
              horizontal-alignment="right"
              [attr.text]="row.value ?? 'Not set'"
              [attr.supporting-text]="row.inherited ? 'Inherited from cluster' : null"
            ></nldd-text-cell>
            <nldd-spacer-cell size="8"></nldd-spacer-cell>
            <nldd-icon-cell
              icon="edit"
              size="20"
            ></nldd-icon-cell>
          </nldd-list-item>
        }
      </nldd-list>
      @if (!$last) {
        <nldd-spacer size="24"></nldd-spacer>
      }
    }
  `,
})
export default class ContainerDefaultsSummaryComponent {
  sections = input.required<DefaultsSummarySection[]>();

  edit = output();
}
