import {
  Component,
  Output,
  EventEmitter,
  inject,
  OnInit,
  OnChanges,
  type SimpleChanges,
  signal,
  Input,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { PLUGIN } from '../../connect/tokens';
import pluginIconSrc from '../utils/plugin-icon';
import {
  ListPluginsRequestSchema,
  ListPresetsRequestSchema,
  type Preset,
} from '../../generated/v1/plugin_pb';

export interface Plugin {
  id: string;
  /** The install identifier ("openfsc"), which names the resource on the
   *  cluster. Not for reading: `displayName` is what a plugin calls itself. */
  name: string;
  displayName: string;
  description: string;
  descriptionShort: string;
  selected: boolean;
}

@Component({
  selector: 'app-shared-plugins-form',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './shared-plugins-form.component.html',
})
export class SharedPluginsFormComponent implements OnInit, OnChanges {
  private pluginClient = inject(PLUGIN);

  @Output() formSubmit = new EventEmitter<{ preset: string; plugins: string[] }>();

  @Input() initialPluginIds?: string[];

  protected readonly pluginIconSrc = pluginIconSrc;

  selectedPreset = 'custom';

  customPluginUploadEnabled = false;

  selectedCustomPluginFile: File | null = null;

  isLoading = signal(true);

  errorMessage = signal<string | null>(null);

  plugins: Plugin[] = [];

  presets: Preset[] = [];

  ngOnInit() {
    this.load();
  }

  async load() {
    this.isLoading.set(true);
    this.errorMessage.set(null);
    try {
      // Fetch plugins and presets from backend
      const [pluginsResponse, presetsResponse] = await Promise.all([
        firstValueFrom(this.pluginClient.listPlugins(create(ListPluginsRequestSchema, {}))),
        firstValueFrom(this.pluginClient.listPresets(create(ListPresetsRequestSchema, {}))),
      ]);

      // Store presets
      this.presets = presetsResponse.presets;

      // Map backend plugins to frontend format
      this.plugins = pluginsResponse.plugins.map((backendPlugin) => ({
        id: backendPlugin.id,
        name: backendPlugin.name,
        displayName: backendPlugin.displayName || backendPlugin.name,
        description: backendPlugin.description,
        descriptionShort: backendPlugin.descriptionShort,
        selected: false,
      }));

      if (this.initialPluginIds) {
        this.applyInitialSelection();
      } else if (this.presets.length > 0) {
        // Nothing to edit, so the form opens on the first preset rather than on
        // an empty cluster nobody wants.
        this.selectedPreset = this.presets[0].id;
        this.onPresetChange();
      }

      this.isLoading.set(false);
    } catch (error) {
      this.errorMessage.set(`Failed to load plugins from server: ${error}`);
      this.isLoading.set(false);
    }
  }

  /** The ids come from a request the parent fires alongside ours, so they can
   *  land before or after our plugin list. Applied from both sides: whichever
   *  arrives last is the one holding both halves. Without this the edit sheet
   *  opens with everything unchecked and saving strips the cluster bare. */
  ngOnChanges(changes: SimpleChanges) {
    if (changes['initialPluginIds'] && this.plugins.length > 0) this.applyInitialSelection();
  }

  private applyInitialSelection() {
    const installed = new Set(this.initialPluginIds ?? []);
    this.plugins = this.plugins.map((plugin) => ({
      ...plugin,
      selected: installed.has(plugin.id),
    }));
    // Shows 'Standard' rather than 'Custom' when the cluster happens to run
    // exactly a preset, same rule as toggling a plugin by hand.
    this.selectedPreset = this.matchingPresetId();
  }

  onPresetRadioChange(event: Event) {
    const detail = (event as CustomEvent<{ selected: boolean; value: string }>).detail;
    // The group also reports the button it deselected. Acting on that would set
    // the preset to the one the user just left.
    if (!detail.selected) return;

    this.selectedPreset = detail.value;
    this.onPresetChange();
  }

  /**
   * A preset is a starting point, not a lock. Toggling a plugin therefore stays
   * possible under any preset; which preset is shown follows from the selection
   * rather than from what was clicked last, so undoing a change by hand puts the
   * preset back instead of leaving you on 'custom' with a selection that is one.
   */
  onPluginToggle(plugin: Plugin, checked: boolean) {
    const target = this.plugins.find((p) => p.id === plugin.id);
    if (target) target.selected = checked;
    this.selectedPreset = this.matchingPresetId();
  }

  /** The preset whose plugins are exactly the current selection, else 'custom'.
   *  Two presets with the same set are indistinguishable here; the first wins. */
  private matchingPresetId(): string {
    const selected = new Set(this.plugins.filter((plugin) => plugin.selected).map((p) => p.id));
    const match = this.presets.find(
      (preset) =>
        preset.pluginIds.length === selected.size &&
        preset.pluginIds.every((id) => selected.has(id)),
    );
    return match ? match.id : 'custom';
  }

  onPresetChange() {
    if (this.selectedPreset === 'custom') {
      // For custom preset, don't change selections automatically
      return;
    }

    // Find the selected preset from backend data
    const preset = this.presets.find((p) => p.id === this.selectedPreset);
    if (!preset) {
      return;
    }

    // Update plugin selections based on preset
    this.plugins = this.plugins.map((plugin) => ({
      ...plugin,
      selected: preset.pluginIds.includes(plugin.id),
    }));
  }

  onCustomPluginUploadToggle(enabled: boolean) {
    this.customPluginUploadEnabled = enabled;
    // Turning it off removes the file field from the page, so a file picked
    // before that would sit in here without anything on screen showing it.
    if (!enabled) this.selectedCustomPluginFile = null;
  }

  // The design system ships Dutch defaults; the console is in US English.
  readonly fileFieldTranslations = {
    'components.file-field.to-choose-file-action': 'Choose file',
    'components.file-field.no-file-chosen-text': 'No file chosen',
    'components.file-field.clear-action': 'Clear selection',
  };

  onCustomPluginFileChange(event: Event) {
    const { files } = (event as CustomEvent<{ files: File[] }>).detail;
    this.selectedCustomPluginFile = files.length > 0 ? files[0] : null;
  }

  submit() {
    this.onSubmit();
  }

  onSubmit(event?: Event) {
    event?.preventDefault();

    const selectedPlugins = this.plugins.filter((plugin) => plugin.selected);

    const data = {
      preset: this.selectedPreset,
      plugins: selectedPlugins.map((plugin) => plugin.id),
    };

    this.formSubmit.emit(data);
  }
}
