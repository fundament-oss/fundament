import {
  Component,
  input,
  output,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { RouterLink, RouterLinkActive } from '@angular/router';
import { NgIcon, provideIcons } from '@ng-icons/core';
import {
  tablerFolder,
  tablerFolders,
  tablerPuzzle,
  tablerUsers,
  tablerChartLine,
  tablerBuilding,
  tablerBracketsContain,
  tablerUserCog,
} from '@ng-icons/tabler-icons';
import { KubernetesIconComponent } from '../icons';
import type { PluginNavGroup } from '../plugin-resources/types';

@Component({
  selector: 'app-sidebar-nav',
  imports: [RouterLink, RouterLinkActive, NgIcon, KubernetesIconComponent],
  viewProviders: [
    provideIcons({
      tablerFolder,
      tablerFolders,
      tablerPuzzle,
      tablerUsers,
      tablerChartLine,
      tablerBuilding,
      tablerBracketsContain,
      tablerUserCog,
    }),
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './sidebar-nav.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class SidebarNavComponent {
  selectedType = input<'organization' | 'project' | null>(null);

  selectedItemDisplay = input<{ type: 'organization' | 'project'; name: string } | null>(null);

  selectedProjectId = input<string | null>(null);

  settingsHeader = input('');

  isClustersActive = input(false);

  organizationNav = input<PluginNavGroup[]>([]);

  projectNav = input<PluginNavGroup[]>([]);

  openSelectorModal = output<void>();

  closeSidebar = output<void>();
}
