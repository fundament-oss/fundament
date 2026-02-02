import { Injectable, Component, Type, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { firstValueFrom } from 'rxjs';

export interface RemoteComponentDefinition {
  id: string;
  name: string;
  version: string;
  type: string;
  template: string;
  styles: string;
  componentClass: string;
  imports: string[];
  metadata: {
    author: string;
    description: string;
    tags: string[];
  };
}

@Injectable({
  providedIn: 'root',
})
export class DynamicComponentLoaderService {
  private http = inject(HttpClient);

  async fetchComponentDefinition(url: string): Promise<RemoteComponentDefinition> {
    return await firstValueFrom(
      this.http.get<RemoteComponentDefinition>(url, {
        headers: { 'Cache-Control': 'no-cache' },
      })
    );
  }

  async compileComponent(definition: RemoteComponentDefinition): Promise<Type<unknown>> {
    const componentClass = this.createComponentClass(definition.componentClass);
    const imports = this.resolveImports(definition.imports);

    return Component({
      selector: `dynamic-${definition.id}`,
      template: definition.template,
      styles: [definition.styles],
      standalone: true,
      imports: imports,
    })(componentClass);
  }

  private createComponentClass(classCode: string): Type<unknown> {
    // WARNING: Uses Function constructor to evaluate code strings
    // In production, validate and sanitize input or use pre-compiled modules
    const classFunction = new Function('signal', `"use strict"; ${classCode} return DynamicPluginComponent;`);
    return classFunction(signal);
  }

  private resolveImports(importNames: string[]): Type<unknown>[] {
    const moduleMap: Record<string, Type<unknown>> = {
      CommonModule: CommonModule,
      FormsModule: FormsModule,
    };

    return importNames
      .map((name) => moduleMap[name])
      .filter((m): m is Type<unknown> => Boolean(m));
  }
}
