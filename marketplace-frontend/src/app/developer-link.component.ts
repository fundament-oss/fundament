import { Component, ChangeDetectionStrategy, inject, input } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { RouterLink } from '@angular/router';
import { ConfigService } from './config.service';

/**
 * A link into the developer portal: by URL in the split deployment (FUN-20),
 * by route in the demo bundle, which carries the manage routes itself, and
 * nothing at all where neither applies, since this build's route table then
 * has no manage routes and the link would only hit its `**` wildcard.
 *
 * Two anchors rather than one carrying both [href] and [routerLink]: RouterLink
 * manages its host's href itself, so the two cannot share an element. The
 * host is `display: contents`, so the anchor is what sits in the parent's
 * layout; classes for it go through `linkClass`.
 */
@Component({
  selector: 'app-developer-link',
  imports: [NgTemplateOutlet, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'contents' },
  template: `
    <ng-template #content><ng-content /></ng-template>
    @if (developerUrl) {
      <a [href]="developerUrl + path()" [class]="linkClass()">
        <ng-container [ngTemplateOutlet]="content" />
      </a>
    } @else if (bundledDeveloperArea) {
      <a [routerLink]="path()" [class]="linkClass()">
        <ng-container [ngTemplateOutlet]="content" />
      </a>
    }
  `,
})
export default class DeveloperLinkComponent {
  private readonly configService = inject(ConfigService);

  /** The portal page, as a root-relative route: `/manage/create`. */
  readonly path = input.required<string>();

  readonly linkClass = input('');

  protected readonly developerUrl = this.configService.getConfig().developerUrl ?? '';

  protected readonly bundledDeveloperArea = this.configService.hasBundledDeveloperArea();
}
