import { Component } from '@angular/core';
import { RouterOutlet } from '@angular/router';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [RouterOutlet],
  template: `
    <h1>Fundament DCIM</h1>
    <router-outlet />
  `,
})
export default class AppComponent {}
