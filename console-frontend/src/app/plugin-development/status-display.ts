import { type PluginStatus } from './plugin-development.service';

// Shared label + tag-color helpers so the hub, detail and version-history
// views render lifecycle status consistently via <nldd-tag>.

export const statusLabel = (status: PluginStatus): string => {
  switch (status) {
    case 'published':
      return 'Published';
    case 'in_review':
      return 'In review';
    case 'changes_requested':
      return 'Changes requested';
    case 'pushed':
      return 'Pushed';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

export const statusTagColor = (
  status: PluginStatus,
): 'success' | 'accent' | 'warning' | 'neutral' => {
  switch (status) {
    case 'published':
      return 'success';
    case 'in_review':
      return 'accent';
    case 'changes_requested':
      return 'warning';
    case 'pushed':
      return 'neutral';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

// Class names for the `.badge` utility (see styles.css) used by the plain
// HTML plugins table, as an alternative to <nldd-tag> for that view.
export const statusBadgeClass = (status: PluginStatus): string => {
  switch (status) {
    case 'published':
      return 'badge-green';
    case 'in_review':
      return 'badge-blue';
    case 'changes_requested':
      return 'badge-orange';
    case 'pushed':
      return 'badge-gray';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};
