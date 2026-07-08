import { type PluginStatus, type PluginTier } from './plugin-development.service';

// Shared label + badge-class helpers so the hub, detail and version-history
// views render lifecycle status and tier consistently.

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

export const statusBadgeClass = (status: PluginStatus): string => {
  switch (status) {
    case 'published':
      return 'badge badge-emerald';
    case 'in_review':
      return 'badge badge-blue';
    case 'changes_requested':
      return 'badge badge-yellow';
    case 'pushed':
      return 'badge badge-gray';
    default:
      throw new Error(`unhandled status: ${status satisfies never}`);
  }
};

export const tierLabel = (tier: PluginTier): string => {
  switch (tier) {
    case 'gold':
      return 'Gold · Built-in';
    case 'silver':
      return 'Silver · Certified';
    case 'bronze':
      return 'Bronze · Experimental';
    case 'grey':
      return 'Grey · Internal';
    default:
      throw new Error(`unhandled tier: ${tier satisfies never}`);
  }
};

export const tierBadgeClass = (tier: PluginTier): string => {
  switch (tier) {
    case 'gold':
      return 'badge badge-yellow';
    case 'silver':
      return 'badge bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200';
    case 'bronze':
      return 'badge bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-200';
    case 'grey':
      return 'badge badge-gray';
    default:
      throw new Error(`unhandled tier: ${tier satisfies never}`);
  }
};
