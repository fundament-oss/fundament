import { Component, Output, EventEmitter, inject, OnInit, signal, Input } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { PLUGIN } from '../../connect/tokens';
import { create } from '@bufbuild/protobuf';
import {
  ListPluginsRequestSchema,
  ListPresetsRequestSchema,
  type Preset,
} from '../../generated/v1/plugin_pb';
import { firstValueFrom } from 'rxjs';

export interface Plugin {
  id: string;
  name: string;
  description: string;
  descriptionShort: string;
  selected: boolean;
}

@Component({
  selector: 'app-shared-plugins-form',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './shared-plugins-form.component.html',
})
export class SharedPluginsFormComponent implements OnInit {
  private pluginClient = inject(PLUGIN);

  @Output() formSubmit = new EventEmitter<{ preset: string; plugins: string[] }>();
  @Input() initialPluginIds?: string[];

  selectedPreset = 'custom';
  customPluginUploadEnabled = false;
  selectedCustomPluginFile: File | null = null;
  isLoading = signal(true);
  errorMessage = signal<string | null>(null);

  plugins: Plugin[] = [];
  presets: Preset[] = [];

  async ngOnInit() {
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
        description: backendPlugin.description,
        descriptionShort: backendPlugin.descriptionShort,
        selected: this.initialPluginIds?.includes(backendPlugin.id) ?? false,
      }));

      // Set default preset if there are presets available
      if (this.presets.length > 0 && !this.initialPluginIds) {
        this.selectedPreset = this.presets[0].id;
        this.onPresetChange();
      }

      this.isLoading.set(false);
    } catch (error) {
      console.error('Failed to load plugins:', error);
      this.errorMessage.set('Failed to load plugins from server');
      this.isLoading.set(false);
    }
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
    this.plugins.forEach((plugin) => {
      plugin.selected = preset.pluginIds.includes(plugin.id);
    });
  }

  onCustomPluginFileChange(event: Event) {
    const input = event.target as HTMLInputElement;
    if (input.files && input.files.length > 0) {
      this.selectedCustomPluginFile = input.files[0];
    } else {
      this.selectedCustomPluginFile = null;
    }
  }

  onSubmit() {
    const selectedPlugins = this.plugins.filter((plugin) => plugin.selected);

    const data = {
      preset: this.selectedPreset,
      plugins: selectedPlugins.map((plugin) => plugin.id),
    };

    this.formSubmit.emit(data);
  }
}
