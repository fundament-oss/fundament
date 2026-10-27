import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  inject,
  input,
} from '@angular/core';
import PageNavService from '../page-nav.service';
import PluginNavService from '../plugin-resources/plugin-nav.service';

import '@nldd/design-system/box';
import '@nldd/design-system/container';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/title';

/**
 * The installed plugins' cluster-scoped resources, as a block on the cluster
 * detail page: their namespaced ones live on the project page instead.
 */
@Component({
  selector: 'app-cluster-plugin-resources-block',
  changeDetection: ChangeDetectionStrategy.OnPush,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  template: `
    @if (pluginNav.clusterNav().length > 0) {
      <nldd-box>
        <nldd-container padding="16">
          <nldd-title size="5">
            <h3>Plugin resources</h3>
          </nldd-title>
          @for (group of pluginNav.clusterNav(); track group.installationName) {
            <nldd-spacer size="16"></nldd-spacer>
            <nldd-title size="6">
              <h4>{{ group.label }}</h4>
            </nldd-title>
            <nldd-spacer size="8"></nldd-spacer>
            <nldd-list type="navigation" [attr.aria-label]="group.label">
              @for (item of group.items; track item.crdPlural) {
                <nldd-list-item
                  [attr.href]="pageNav.path(resourcePath(group.installationName, item.crdPlural))"
                  (click)="
                    pageNav.navigate($event, resourcePath(group.installationName, item.crdPlural))
                  "
                >
                  <nldd-icon-cell
                    [attr.icon]="item.icon || 'circle-dashed'"
                    size="20"
                  ></nldd-icon-cell>
                  <nldd-spacer-cell size="8"></nldd-spacer-cell>
                  <nldd-text-cell [attr.text]="item.label"></nldd-text-cell>
                </nldd-list-item>
              }
            </nldd-list>
          }
        </nldd-container>
      </nldd-box>
    }
  `,
})
export default class ClusterPluginResourcesBlockComponent {
  clusterId = input.required<string>();

  protected pageNav = inject(PageNavService);

  protected pluginNav = inject(PluginNavService);

  protected resourcePath(installationName: string, crdPlural: string): string {
    return `/clusters/${this.clusterId()}/plugin-resources/${installationName}/${crdPlural}`;
  }
}
