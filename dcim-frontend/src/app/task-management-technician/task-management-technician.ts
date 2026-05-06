import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  signal,
  computed,
  inject,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';

interface GatherItem {
  label: string;
  taskFor?: string;
}

interface Step {
  title: string;
  description: string;
  icon: string;
  svg: string;
}

interface Task {
  title: string;
  priority: 'critical' | 'high' | 'normal';
  location: string;
  steps: Step[];
}

type Phase = 'gather' | 'task';

@Component({
  selector: 'app-task-management-technician',
  templateUrl: './task-management-technician.html',
  imports: [RouterLink],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'block bg-neutral-50 font-sans text-neutral-900 antialiased' },
})
export default class TaskManagementTechnicianComponent {
  private sanitizer = inject(DomSanitizer);

  readonly dcName = 'DC Amsterdam-West';

  readonly gatherItems: GatherItem[] = [
    { label: 'Anti-static wrist strap' },
    { label: 'Phillips-head screwdriver' },
    { label: 'Multimeter' },
    { label: 'Seagate Exos X18, 16 TB', taskFor: 'Replace broken harddisk — Rack 123' },
    { label: 'Cisco Catalyst 9200L switch', taskFor: 'Replace network switch — Rack 87' },
  ];

  readonly tasks: Task[] = [
    {
      title: 'Replace broken harddisk',
      priority: 'critical',
      location: `${this.dcName} · Rack 123`,
      steps: [
        {
          title: 'Navigate to data center Hall B',
          description:
            'Head to Hall B via the main corridor. Follow the blue floor markers. Your destination is Row 12, approximately halfway down the hall on the left side.',
          icon: 'info-circle',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="10" y="20" width="100" height="60" rx="6" stroke="#e2e8f0" stroke-width="1.5" fill="#f8fafc"/>
            <rect x="18" y="28" width="30" height="44" rx="3" stroke="#cbd5e1" stroke-width="1" fill="white"/>
            <text x="33" y="42" text-anchor="middle" fill="#94a3b8" font-size="7" font-weight="600">Hall A</text>
            <rect x="56" y="28" width="30" height="44" rx="3" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <text x="71" y="42" text-anchor="middle" fill="#6366f1" font-size="7" font-weight="600">Hall B</text>
            <line x1="62" y1="50" x2="62" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <line x1="68" y1="50" x2="68" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <line x1="74" y1="50" x2="74" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <line x1="80" y1="50" x2="80" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <circle cx="68" cy="58" r="4" fill="#6366f1"/>
            <circle cx="68" cy="58" r="2" fill="white"/>
            <path d="M48 50 L56 50" stroke="#6366f1" stroke-width="2" stroke-dasharray="3 2"/>
            <text x="71" y="54" text-anchor="middle" fill="#4f46e5" font-size="5">Row 12</text>
          </svg>`,
        },
        {
          title: 'Enter the cold aisle',
          description:
            'Use your access badge on the card reader to enter the cold aisle between Row 12 and Row 13. The door will lock behind you automatically. Ensure the aisle containment doors are properly sealed after entry.',
          icon: 'arrow-right',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="25" y="15" width="35" height="65" rx="4" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <rect x="30" y="20" width="25" height="55" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
            <circle cx="50" cy="48" r="2.5" fill="#6366f1"/>
            <rect x="70" y="30" width="22" height="32" rx="4" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <rect x="74" y="36" width="14" height="8" rx="2" fill="#a5b4fc"/>
            <rect x="74" y="48" width="14" height="8" rx="2" fill="#c7d2fe"/>
            <path d="M60 48 L70 42" stroke="#6366f1" stroke-width="1.5" stroke-dasharray="3 2"/>
            <path d="M76 85 Q81 80 86 85" stroke="#22c55e" stroke-width="2" fill="none"/>
            <circle cx="81" cy="88" r="1" fill="#22c55e"/>
          </svg>`,
        },
        {
          title: 'Locate Rack 123',
          description:
            'Rack 123 is on the left side of the aisle, the 4th rack from the entrance. It has a label plate reading "R-123" at the top. Verify the rack number before proceeding.',
          icon: 'database',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="10" y="15" width="18" height="70" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f8fafc"/>
            <rect x="30" y="15" width="18" height="70" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f8fafc"/>
            <rect x="50" y="15" width="18" height="70" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f8fafc"/>
            <rect x="70" y="12" width="22" height="76" rx="3" stroke="#6366f1" stroke-width="2.5" fill="#eef2ff"/>
            <rect x="74" y="20" width="14" height="6" rx="1" fill="#a5b4fc"/>
            <rect x="74" y="30" width="14" height="6" rx="1" fill="#a5b4fc"/>
            <rect x="74" y="40" width="14" height="6" rx="1" fill="#a5b4fc"/>
            <rect x="74" y="50" width="14" height="6" rx="1" fill="#c7d2fe"/>
            <rect x="74" y="60" width="14" height="6" rx="1" fill="#c7d2fe"/>
            <text x="81" y="10" text-anchor="middle" fill="#6366f1" font-size="6" font-weight="700">R-123</text>
            <rect x="94" y="15" width="18" height="70" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f8fafc"/>
            <path d="M81 92 L81 88" stroke="#6366f1" stroke-width="2"/>
            <polygon points="76,92 86,92 81,97" fill="#6366f1"/>
          </svg>`,
        },
        {
          title: 'Open the rack',
          description:
            "Enter access code 4591 on the rack's keypad lock. The lock indicator LED will turn green. Pull the handle to open the front door. Keep the door open during the procedure.",
          icon: 'lock-open',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="35" y="20" width="50" height="55" rx="6" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <rect x="43" y="35" width="12" height="10" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
            <rect x="59" y="35" width="12" height="10" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
            <rect x="43" y="49" width="12" height="10" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
            <rect x="59" y="49" width="12" height="10" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
            <text x="49" y="43" text-anchor="middle" fill="#6366f1" font-size="7" font-weight="600">4</text>
            <text x="65" y="43" text-anchor="middle" fill="#6366f1" font-size="7" font-weight="600">5</text>
            <text x="49" y="57" text-anchor="middle" fill="#6366f1" font-size="7" font-weight="600">9</text>
            <text x="65" y="57" text-anchor="middle" fill="#6366f1" font-size="7" font-weight="600">1</text>
            <circle cx="75" cy="28" r="4" fill="#22c55e"/>
            <path d="M73 28 l2 2 l3-4" stroke="white" stroke-width="1.5" fill="none"/>
          </svg>`,
        },
        {
          title: 'Locate device "backup-srv-07" at U32',
          description:
            'Count rack units from the bottom. U32 is in the upper third of the rack. The server is labeled "backup-srv-07" on a pull-out tag on the left side. It\'s a 2U server with a dark gray bezel.',
          icon: 'search',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="30" y="8" width="60" height="84" rx="4" stroke="#cbd5e1" stroke-width="1.5" fill="white"/>
            <rect x="35" y="14" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <rect x="35" y="24" width="50" height="7" rx="1.5" fill="#eef2ff" stroke="#6366f1" stroke-width="1.5"/>
            <text x="42" y="30" fill="#6366f1" font-size="5" font-weight="600">backup-srv-07</text>
            <rect x="35" y="34" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <rect x="35" y="44" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <rect x="35" y="54" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <rect x="35" y="64" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <rect x="35" y="74" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <text x="28" y="30" text-anchor="end" fill="#6366f1" font-size="5" font-weight="600">U32</text>
            <path d="M22 28 L30 28" stroke="#6366f1" stroke-width="1.5"/>
          </svg>`,
        },
        {
          title: 'Remove failed harddisk (Bay 3, top-left)',
          description:
            "Put on your anti-static wrist strap and ground yourself. Locate Bay 3 at the top-left of the server's drive cage. Press the orange release latch and slide the caddy out gently. Place the failed drive in the anti-static bag.",
          icon: 'cylinder-split-slash',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="20" y="25" width="60" height="50" rx="4" stroke="#cbd5e1" stroke-width="1.5" fill="white"/>
            <rect x="26" y="31" width="22" height="16" rx="2" stroke="#ef4444" stroke-width="2" fill="#fef2f2" stroke-dasharray="4 2"/>
            <text x="37" y="42" text-anchor="middle" fill="#ef4444" font-size="6" font-weight="600">Bay 3</text>
            <rect x="52" y="31" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
            <rect x="26" y="52" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
            <rect x="52" y="52" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
            <path d="M37 35 L37 20 L95 20 L95 55" stroke="#6366f1" stroke-width="1.5" stroke-dasharray="4 2"/>
            <rect x="85" y="40" width="22" height="35" rx="3" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <rect x="89" y="46" width="14" height="3" rx="1" fill="#a5b4fc"/>
            <rect x="89" y="52" width="14" height="3" rx="1" fill="#a5b4fc"/>
            <circle cx="96" cy="67" r="4" stroke="#a5b4fc" stroke-width="1.5" fill="none"/>
            <circle cx="96" cy="67" r="1" fill="#a5b4fc"/>
          </svg>`,
        },
        {
          title: 'Install replacement harddisk',
          description:
            'Take the new Seagate Exos X18 out of its packaging. Align the drive caddy with Bay 3 rails and slide it in firmly until it clicks into place. The activity LED should blink amber briefly, then turn solid green.',
          icon: 'cylinder-split',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="20" y="25" width="60" height="50" rx="4" stroke="#cbd5e1" stroke-width="1.5" fill="white"/>
            <rect x="26" y="31" width="22" height="16" rx="2" stroke="#22c55e" stroke-width="2" fill="#f0fdf4"/>
            <text x="37" y="42" text-anchor="middle" fill="#22c55e" font-size="6" font-weight="600">Bay 3</text>
            <rect x="52" y="31" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
            <rect x="26" y="52" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
            <rect x="52" y="52" width="22" height="16" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f1f5f9"/>
            <path d="M95 55 L95 20 L37 20 L37 31" stroke="#6366f1" stroke-width="1.5" stroke-dasharray="4 2"/>
            <polygon points="34,30 37,25 40,30" fill="#6366f1"/>
            <rect x="85" y="40" width="22" height="35" rx="3" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <rect x="89" y="46" width="14" height="3" rx="1" fill="#a5b4fc"/>
            <rect x="89" y="52" width="14" height="3" rx="1" fill="#a5b4fc"/>
            <circle cx="96" cy="67" r="4" stroke="#a5b4fc" stroke-width="1.5" fill="none"/>
            <circle cx="96" cy="67" r="1" fill="#a5b4fc"/>
            <circle cx="30" cy="29" r="3" fill="#22c55e"/>
          </svg>`,
        },
        {
          title: 'Verify & close up',
          description:
            'Wait 30 seconds for the RAID controller to detect the new drive. The status LED on Bay 3 should be solid green. Check the server\'s front LCD panel — it should show "Rebuild in progress" or "Drive OK". Close and lock the rack door.',
          icon: 'check-mark-circle',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="25" y="15" width="70" height="50" rx="6" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <rect x="32" y="22" width="56" height="30" rx="3" fill="white" stroke="#a5b4fc" stroke-width="1"/>
            <text x="60" y="34" text-anchor="middle" fill="#6366f1" font-size="5.5" font-weight="500">RAID Status</text>
            <rect x="40" y="39" width="40" height="6" rx="3" fill="#dcfce7"/>
            <rect x="40" y="39" width="32" height="6" rx="3" fill="#22c55e"/>
            <text x="60" y="44" text-anchor="middle" fill="white" font-size="4" font-weight="700">Rebuilding 80%</text>
            <circle cx="35" cy="72" r="5" fill="#22c55e"/>
            <path d="M33 72 l2 2 l3-4" stroke="white" stroke-width="1.5" fill="none"/>
            <text x="44" y="74" fill="#16a34a" font-size="5.5" font-weight="600">Drive OK — Bay 3</text>
            <rect x="25" y="80" width="70" height="8" rx="2" stroke="#cbd5e1" stroke-width="1" fill="#f8fafc"/>
            <rect x="29" y="82" width="4" height="4" rx="1" fill="#22c55e"/>
            <rect x="36" y="82" width="4" height="4" rx="1" fill="#22c55e"/>
            <rect x="43" y="82" width="4" height="4" rx="1" fill="#e2e8f0"/>
            <rect x="50" y="82" width="4" height="4" rx="1" fill="#e2e8f0"/>
          </svg>`,
        },
      ],
    },
    {
      title: 'Replace network switch',
      priority: 'high',
      location: `${this.dcName} · Rack 87`,
      steps: [
        {
          title: 'Navigate to Rack 87',
          description:
            'Head to Row 9 in Hall B. Rack 87 is on the right side of the aisle, the 2nd rack from the entrance. The label plate reads "R-087".',
          icon: 'info-circle',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="10" y="20" width="100" height="60" rx="6" stroke="#e2e8f0" stroke-width="1.5" fill="#f8fafc"/>
            <rect x="18" y="28" width="30" height="44" rx="3" stroke="#cbd5e1" stroke-width="1" fill="white"/>
            <text x="33" y="42" text-anchor="middle" fill="#94a3b8" font-size="7" font-weight="600">Hall A</text>
            <rect x="56" y="28" width="30" height="44" rx="3" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <text x="71" y="42" text-anchor="middle" fill="#6366f1" font-size="7" font-weight="600">Hall B</text>
            <line x1="62" y1="50" x2="62" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <line x1="68" y1="50" x2="68" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <line x1="74" y1="50" x2="74" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <circle cx="65" cy="55" r="4" fill="#6366f1"/>
            <circle cx="65" cy="55" r="2" fill="white"/>
            <text x="72" y="51" fill="#4f46e5" font-size="5">Row 9</text>
          </svg>`,
        },
        {
          title: 'Open rack & locate switch at U18',
          description:
            'Enter code 7823 on the keypad. U18 holds a 1U Cisco switch labeled "sw-core-03". It has an amber status LED — this is the failed unit.',
          icon: 'lock-open',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="30" y="8" width="60" height="84" rx="4" stroke="#cbd5e1" stroke-width="1.5" fill="white"/>
            <rect x="35" y="14" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <rect x="35" y="24" width="50" height="7" rx="1.5" fill="#fef3c7" stroke="#f59e0b" stroke-width="1.5"/>
            <text x="42" y="30" fill="#b45309" font-size="5" font-weight="600">sw-core-03 ⚠</text>
            <rect x="35" y="34" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <rect x="35" y="44" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <rect x="35" y="54" width="50" height="7" rx="1.5" fill="#f1f5f9"/>
            <text x="28" y="30" text-anchor="end" fill="#6366f1" font-size="5" font-weight="600">U18</text>
            <path d="M22 28 L30 28" stroke="#6366f1" stroke-width="1.5"/>
          </svg>`,
        },
        {
          title: 'Remove failed switch',
          description:
            'Label all connected cables with the provided tags before disconnecting. Unscrew the rack ears (2 screws each side) and slide the switch forward. Place in the anti-static bag.',
          icon: 'cylinder-split-slash',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="20" y="35" width="60" height="14" rx="2" stroke="#ef4444" stroke-width="2" fill="#fef2f2" stroke-dasharray="4 2"/>
            <text x="50" y="45" text-anchor="middle" fill="#ef4444" font-size="6" font-weight="600">sw-core-03</text>
            <path d="M30 49 L30 62" stroke="#cbd5e1" stroke-width="2"/>
            <path d="M42 49 L42 62" stroke="#cbd5e1" stroke-width="2"/>
            <path d="M54 49 L54 62" stroke="#cbd5e1" stroke-width="2"/>
            <path d="M66 49 L66 62" stroke="#cbd5e1" stroke-width="2"/>
            <path d="M95 42 L85 42" stroke="#6366f1" stroke-width="1.5" stroke-dasharray="3 2"/>
            <polygon points="87,39 82,42 87,45" fill="#6366f1"/>
          </svg>`,
        },
        {
          title: 'Install Cisco Catalyst 9200L',
          description:
            'Slide the new switch into U18. Secure with rack ear screws. Re-connect cables in the order matching your labels. The switch will power on and run POST diagnostics.',
          icon: 'list',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="20" y="35" width="60" height="14" rx="2" stroke="#22c55e" stroke-width="2" fill="#f0fdf4"/>
            <text x="50" y="45" text-anchor="middle" fill="#16a34a" font-size="6" font-weight="600">Catalyst 9200L</text>
            <path d="M30 49 L30 62" stroke="#6366f1" stroke-width="2"/>
            <path d="M42 49 L42 62" stroke="#6366f1" stroke-width="2"/>
            <path d="M54 49 L54 62" stroke="#6366f1" stroke-width="2"/>
            <path d="M66 49 L66 62" stroke="#6366f1" stroke-width="2"/>
            <circle cx="30" cy="33" r="2" fill="#22c55e"/>
            <circle cx="42" cy="33" r="2" fill="#22c55e"/>
            <circle cx="54" cy="33" r="2" fill="#22c55e"/>
            <path d="M82 42 L92 42" stroke="#6366f1" stroke-width="1.5" stroke-dasharray="3 2"/>
            <polygon points="90,39 95,42 90,45" fill="#6366f1"/>
          </svg>`,
        },
        {
          title: 'Verify connectivity & close rack',
          description:
            'Wait 2 minutes for the switch to boot. All port LEDs should turn green. Confirm "sw-core-03" is back online on the NOC dashboard. Close and lock the rack.',
          icon: 'check-mark-circle',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="25" y="15" width="70" height="50" rx="6" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <rect x="32" y="22" width="56" height="30" rx="3" fill="white" stroke="#a5b4fc" stroke-width="1"/>
            <text x="60" y="32" text-anchor="middle" fill="#6366f1" font-size="5.5" font-weight="500">NOC Dashboard</text>
            <circle cx="45" cy="43" r="4" fill="#22c55e"/>
            <path d="M43 43 l2 2 l3-4" stroke="white" stroke-width="1.5" fill="none"/>
            <text x="54" y="46" fill="#16a34a" font-size="5" font-weight="600">sw-core-03 online</text>
            <circle cx="35" cy="72" r="5" fill="#22c55e"/>
            <path d="M33 72 l2 2 l3-4" stroke="white" stroke-width="1.5" fill="none"/>
            <text x="44" y="74" fill="#16a34a" font-size="5.5" font-weight="600">All ports active</text>
          </svg>`,
        },
      ],
    },
    {
      title: 'Inspect PDU',
      priority: 'normal',
      location: `${this.dcName} · Hall A`,
      steps: [
        {
          title: 'Navigate to PDU — Hall A, Row 3',
          description:
            'Head to Hall A via the main corridor. The PDU is a vertical unit mounted on the right side of Rack 42, Row 3. It\'s labeled "PDU-A-042".',
          icon: 'info-circle',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="10" y="20" width="100" height="60" rx="6" stroke="#e2e8f0" stroke-width="1.5" fill="#f8fafc"/>
            <rect x="18" y="28" width="30" height="44" rx="3" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <text x="33" y="42" text-anchor="middle" fill="#6366f1" font-size="7" font-weight="600">Hall A</text>
            <line x1="24" y1="50" x2="24" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <line x1="30" y1="50" x2="30" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <line x1="36" y1="50" x2="36" y2="68" stroke="#a5b4fc" stroke-width="1" stroke-dasharray="2 2"/>
            <circle cx="30" cy="58" r="4" fill="#6366f1"/>
            <circle cx="30" cy="58" r="2" fill="white"/>
            <text x="37" y="54" fill="#4f46e5" font-size="5">Row 3</text>
            <rect x="56" y="28" width="30" height="44" rx="3" stroke="#cbd5e1" stroke-width="1" fill="white"/>
            <text x="71" y="42" text-anchor="middle" fill="#94a3b8" font-size="7" font-weight="600">Hall B</text>
          </svg>`,
        },
        {
          title: 'Record power load readings',
          description:
            "Use the multimeter to measure input voltage on all three phases. Expected: 220–240V each. Note any circuit above 80% capacity on the PDU's LCD display.",
          icon: 'exclamation-triangle',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="40" y="10" width="40" height="55" rx="4" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <rect x="45" y="18" width="30" height="16" rx="2" fill="white" stroke="#a5b4fc" stroke-width="1"/>
            <text x="60" y="25" text-anchor="middle" fill="#6366f1" font-size="5" font-weight="600">PDU-A-042</text>
            <text x="60" y="31" text-anchor="middle" fill="#334155" font-size="5">231V  18.4A</text>
            <rect x="45" y="38" width="8" height="12" rx="1.5" fill="#22c55e"/>
            <rect x="56" y="38" width="8" height="12" rx="1.5" fill="#22c55e"/>
            <rect x="67" y="38" width="8" height="12" rx="1.5" fill="#f59e0b"/>
            <text x="49" y="48" text-anchor="middle" fill="white" font-size="4">L1</text>
            <text x="60" y="48" text-anchor="middle" fill="white" font-size="4">L2</text>
            <text x="71" y="48" text-anchor="middle" fill="white" font-size="4">L3</text>
            <circle cx="60" cy="76" r="10" stroke="#6366f1" stroke-width="2" fill="#f8fafc"/>
            <path d="M55 76 L59 72 L61 76 L65 70" stroke="#6366f1" stroke-width="1.5" fill="none"/>
          </svg>`,
        },
        {
          title: 'Inspect cable management & outlets',
          description:
            'Check for loose cables, damaged outlets, or signs of heat stress (discolouration, melting). Verify all outlet covers are in place on unused ports. Note any anomalies.',
          icon: 'info-circle',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="40" y="10" width="40" height="70" rx="4" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <circle cx="60" cy="28" r="8" stroke="#6366f1" stroke-width="1.5" fill="white"/>
            <rect x="57" y="24" width="2.5" height="5" rx="1" fill="#6366f1"/>
            <rect x="62" y="24" width="2.5" height="5" rx="1" fill="#6366f1"/>
            <circle cx="60" cy="28" r="1.5" fill="#6366f1"/>
            <circle cx="60" cy="48" r="8" stroke="#6366f1" stroke-width="1.5" fill="white"/>
            <rect x="57" y="44" width="2.5" height="5" rx="1" fill="#6366f1"/>
            <rect x="62" y="44" width="2.5" height="5" rx="1" fill="#6366f1"/>
            <circle cx="60" cy="48" r="1.5" fill="#6366f1"/>
            <circle cx="60" cy="67" r="8" stroke="#22c55e" stroke-width="1.5" fill="#f0fdf4"/>
            <path d="M57 67 l3 3 l4-5" stroke="#22c55e" stroke-width="1.5" fill="none"/>
          </svg>`,
        },
        {
          title: 'Document findings & close',
          description:
            'Record all readings and observations. If any circuit is above 80% load or anomalies were found, flag the issue in the system before leaving.',
          icon: 'check-mark-circle',
          svg: `<svg viewBox="0 0 120 100" fill="none" xmlns="http://www.w3.org/2000/svg" class="h-40 w-full" aria-hidden="true">
            <rect x="35" y="10" width="50" height="65" rx="6" stroke="#6366f1" stroke-width="2" fill="#eef2ff"/>
            <path d="M55 16 h10 v4 a2 2 0 0 1-2 2h-6a2 2 0 0 1-2-2z" fill="#6366f1"/>
            <circle cx="60" cy="14" r="3" fill="#6366f1"/>
            <line x1="45" y1="34" x2="49" y2="38" stroke="#22c55e" stroke-width="1.5"/>
            <line x1="49" y1="38" x2="55" y2="30" stroke="#22c55e" stroke-width="1.5"/>
            <rect x="59" y="32" width="18" height="2.5" rx="1" fill="#a5b4fc"/>
            <line x1="45" y1="46" x2="49" y2="50" stroke="#22c55e" stroke-width="1.5"/>
            <line x1="49" y1="50" x2="55" y2="42" stroke="#22c55e" stroke-width="1.5"/>
            <rect x="59" y="44" width="14" height="2.5" rx="1" fill="#a5b4fc"/>
            <rect x="45" y="56" width="28" height="2.5" rx="1" fill="#c7d2fe"/>
            <rect x="45" y="62" width="20" height="2.5" rx="1" fill="#c7d2fe"/>
          </svg>`,
        },
      ],
    },
  ];

  // ── State signals ──
  readonly phase = signal<Phase>('gather');
  readonly currentTaskIndex = signal(0);
  readonly currentStepIndex = signal(0);
  readonly checkedItems = signal(new Set<number>());
  readonly gatherCompleted = signal(false);
  readonly showCompleteScreen = signal(false);
  readonly menuOpen = signal(false);
  readonly showPhotoModal = signal(false);
  readonly showNoteModal = signal(false);
  readonly photoPreviewUrl = signal<string | null>(null);
  readonly toastMessage = signal<string | null>(null);

  private toastTimeout: ReturnType<typeof setTimeout> | undefined;
  private completedTaskSteps = new Map<number, Set<number>>();

  // ── Computed ──
  readonly currentTask = computed(() => this.tasks[this.currentTaskIndex()]);

  readonly totalSteps = computed(
    () => 1 + this.tasks.reduce((s, t) => s + t.steps.length, 0),
  );

  readonly completedCount = computed(() => {
    let n = this.gatherCompleted() ? 1 : 0;
    this.completedTaskSteps.forEach((s) => {
      n += s.size;
    });
    return n;
  });

  readonly progressPct = computed(
    () => (this.completedCount() / this.totalSteps()) * 100,
  );

  readonly showCompleteBtn = computed(() => {
    const p = this.phase();
    if (p !== 'task') return false;
    const ti = this.currentTaskIndex();
    const si = this.currentStepIndex();
    return ti === this.tasks.length - 1 && si === this.tasks[ti].steps.length - 1;
  });

  readonly currentStepLabel = computed(() => {
    if (this.phase() === 'gather') return 'Gather tools & parts';
    const task = this.tasks[this.currentTaskIndex()];
    const si = this.currentStepIndex();
    return `${task.title} — Step ${si + 1}: ${task.steps[si].title}`;
  });

  // ── Methods ──
  toggleMenu(event: Event): void {
    event.stopPropagation();
    this.menuOpen.update((v) => !v);
  }

  isTaskActive(taskIdx: number): boolean {
    return this.phase() === 'task' && this.currentTaskIndex() === taskIdx;
  }

  isTaskDone(taskIdx: number): boolean {
    const task = this.tasks[taskIdx];
    const completedSet = this.completedTaskSteps.get(taskIdx);
    return (completedSet?.size ?? 0) === task.steps.length && task.steps.length > 0;
  }

  isStepActive(taskIdx: number, stepIdx: number): boolean {
    return this.isTaskActive(taskIdx) && this.currentStepIndex() === stepIdx;
  }

  isStepDone(taskIdx: number, stepIdx: number): boolean {
    return this.completedTaskSteps.get(taskIdx)?.has(stepIdx) ?? false;
  }

  jumpToStep(taskIdx: number, stepIdx: number): void {
    this.phase.set('task');
    this.currentTaskIndex.set(taskIdx);
    this.currentStepIndex.set(stepIdx);
  }

  safeSvg(svg: string): SafeHtml {
    return this.sanitizer.bypassSecurityTrustHtml(svg);
  }

  toggleGatherItem(index: number): void {
    this.checkedItems.update((set) => {
      const next = new Set(set);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  }

  onGatherCheckbox(index: number, checked: boolean): void {
    this.checkedItems.update((set) => {
      const next = new Set(set);
      if (checked) next.add(index);
      else next.delete(index);
      return next;
    });
  }

  pressPrev(): void {
    if (this.phase() === 'gather') return;
    if (this.currentStepIndex() > 0) {
      this.currentStepIndex.update((v) => v - 1);
    } else if (this.currentTaskIndex() > 0) {
      this.currentTaskIndex.update((v) => v - 1);
      this.currentStepIndex.set(this.tasks[this.currentTaskIndex()].steps.length - 1);
    } else {
      this.phase.set('gather');
    }
  }

  pressDone(): void {
    if (this.phase() === 'gather') {
      if (this.checkedItems().size < this.gatherItems.length) {
        this.showToast(
          `${this.checkedItems().size}/${this.gatherItems.length} items checked — proceeding`,
        );
      }
      this.gatherCompleted.set(true);
      this.phase.set('task');
      this.currentTaskIndex.set(0);
      this.currentStepIndex.set(0);
      return;
    }

    const ti = this.currentTaskIndex();
    const si = this.currentStepIndex();
    if (!this.completedTaskSteps.has(ti)) {
      this.completedTaskSteps.set(ti, new Set());
    }
    this.completedTaskSteps.get(ti)!.add(si);

    // Force reactivity on completedCount
    this.gatherCompleted.update((v) => v);

    const task = this.tasks[ti];
    if (si < task.steps.length - 1) {
      this.currentStepIndex.update((v) => v + 1);
    } else if (ti < this.tasks.length - 1) {
      this.currentTaskIndex.update((v) => v + 1);
      this.currentStepIndex.set(0);
    } else {
      this.showCompleteScreen.set(true);
    }
  }

  openPhotoModal(): void {
    this.photoPreviewUrl.set(null);
    this.showPhotoModal.set(true);
  }

  closePhotoModal(): void {
    this.showPhotoModal.set(false);
  }

  savePhoto(): void {
    this.showPhotoModal.set(false);
    this.showToast('Photo saved');
  }

  onPhotoSelected(event: Event): void {
    const file = (event.target as HTMLInputElement).files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (ev) => {
        this.photoPreviewUrl.set(ev.target!.result as string);
      };
      reader.readAsDataURL(file);
    }
  }

  openNoteModal(): void {
    this.showNoteModal.set(true);
  }

  closeNoteModal(): void {
    this.showNoteModal.set(false);
  }

  saveNote(): void {
    this.showNoteModal.set(false);
    this.showToast('Note saved');
  }

  onModalBackdropClick(event: Event, modal: 'photo' | 'note'): void {
    if (event.target === event.currentTarget) {
      if (modal === 'photo') this.showPhotoModal.set(false);
      else this.showNoteModal.set(false);
    }
  }

  private showToast(msg: string): void {
    this.toastMessage.set(msg);
    clearTimeout(this.toastTimeout);
    this.toastTimeout = setTimeout(() => {
      this.toastMessage.set(null);
    }, 2500);
  }
}
