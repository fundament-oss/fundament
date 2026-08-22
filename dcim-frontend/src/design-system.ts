// Per-component subpath imports instead of the package root: the root barrel
// pulls a single 2.6 MB chunk with all ~110 components (including CodeMirror,
// which we never use). Keep this list in sync with the <nldd-*> tags in src/ --
// sub-elements ship with their parent (nldd-table-row via ./table,
// nldd-menu-item via ./menu, nldd-form-field-help-text via ./form-field, etc.).
//
// Its own module rather than the entrypoint: the demo build boots from
// main.demo.ts, and two lists of the same components would drift apart.
import '@nldd/design-system/app-view';
import '@nldd/design-system/avatar';
import '@nldd/design-system/activity-indicator';
import '@nldd/design-system/badge';
import '@nldd/design-system/bar-split-view';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/button-group';
import '@nldd/design-system/card';
import '@nldd/design-system/cell';
import '@nldd/design-system/checkbox';
import '@nldd/design-system/checkbox-field';
import '@nldd/design-system/collection';
import '@nldd/design-system/combo-box';
import '@nldd/design-system/container';
import '@nldd/design-system/date-field';
import '@nldd/design-system/dropdown';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/form-section';
import '@nldd/design-system/icon';
import '@nldd/design-system/icon-button';
import '@nldd/design-system/icon-cell';
import '@nldd/design-system/identity';
import '@nldd/design-system/image';
import '@nldd/design-system/inline-dialog';
import '@nldd/design-system/link';
import '@nldd/design-system/list';
import '@nldd/design-system/list-item';
import '@nldd/design-system/list-item-segment';
import '@nldd/design-system/menu';
import '@nldd/design-system/modal-dialog';
import '@nldd/design-system/multi-line-text-field';
import '@nldd/design-system/navigation-split-view';
import '@nldd/design-system/notification';
import '@nldd/design-system/number-field';
import '@nldd/design-system/page';
import '@nldd/design-system/page-footer';
import '@nldd/design-system/password-field';
import '@nldd/design-system/popover';
import '@nldd/design-system/progress-bar';
import '@nldd/design-system/radio-button-field';
import '@nldd/design-system/radio-button-group';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/search-field';
import '@nldd/design-system/segmented-control';
import '@nldd/design-system/sheet';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/spacer-cell';
import '@nldd/design-system/split-view-pane';
import '@nldd/design-system/table';
import '@nldd/design-system/tag';
import '@nldd/design-system/text';
import '@nldd/design-system/text-cell';
import '@nldd/design-system/text-field';
import '@nldd/design-system/timeline-track-cell';
import '@nldd/design-system/title';
import '@nldd/design-system/title-cell';
import '@nldd/design-system/toggle-button';
import '@nldd/design-system/toggle-button-group';
import '@nldd/design-system/tooltip';
import '@nldd/design-system/token-field';
import '@nldd/design-system/toolbar';
import '@nldd/design-system/top-title-bar';
