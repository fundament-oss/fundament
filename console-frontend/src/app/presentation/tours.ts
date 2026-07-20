import { DriveStep, Slide, Tour } from './presentation.model';

// Tours are the walkthrough's content. The chooser groups them into "verhalen"
// (no persona) and "word een rol" (with a persona).
//
// Every `route` below must exist in src/app/app.routes.ts *and* be served by the
// demo transport (src/app/demo/mock-transport.ts); cluster/project ids must match
// src/app/demo/fixtures.ts. A route whose RPCs the mock transport doesn't answer
// renders an error pane mid-presentation, so stick to what is already stubbed.

const CONSOLE_LINK = {
  url: 'https://console.fundament.projects.digilab.network/',
  label: 'console.fundament.projects.digilab.network',
};

const KEYS_ASIDE =
  'Gebruik de pijltjestoetsen ← → om door de slides te navigeren. Esc gaat terug naar de keuze.';

/** Types a cluster name into the wizard and submits step 1. */
const addClusterDrive: DriveStep[] = [
  { wait: 900 },
  { set: '#clusterName', value: 'burgerzaken-acc', type: true },
  { wait: 700 },
  { submit: 'nldd-form' },
];

/**
 * Installs the first plugin in the catalog (cert-manager) on every eligible cluster.
 * Selectors lean on static attributes only — `variant`/`slot` are plain attributes in
 * the template, while `text` is an Angular binding and never lands in the DOM.
 * The "select all" checkbox is the modal's first, and renders when >1 cluster is eligible.
 */
const installPluginDrive: DriveStep[] = [
  { wait: 1400 },
  { click: 'div.grid > div:nth-child(1) nldd-button[variant="primary"]' },
  { wait: 1000 },
  { set: 'nldd-modal-dialog nldd-checkbox-field', check: true },
  { wait: 800 },
  { click: 'nldd-modal-dialog nldd-button[slot="actions"][variant="primary"]' },
];

const closing = (lead: string): Slide => ({
  id: 'closing',
  kind: 'closing',
  full: true,
  title: 'Zelf proberen?',
  lead,
  link: CONSOLE_LINK,
});

// Icons for the chooser cards: SVG path `d`, 24×24 viewBox, stroked.
const ICONS = {
  compass: 'M12 3a9 9 0 100 18 9 9 0 000-18zM15.5 8.5l-2 5-5 2 2-5 5-2z',
  terminal: 'M4 17l6-5-6-5M12 19h8',
  layers: 'M12 3l9 5-9 5-9-5 9-5M3 14l9 5 9-5',
  shield: 'M12 3l7 3v6c0 4.5-3 7.5-7 9-4-1.5-7-4.5-7-9V6l7-3z',
  building: 'M4 21V7l8-4 8 4v14M9 21v-5h6v5M8 11h.01M12 11h.01M16 11h.01',
};

// --- Verhalen -------------------------------------------------------------

const wholeStory: Tour = {
  id: 'clusters-projects',
  title: 'Het hele verhaal',
  lead: 'Van overzicht tot het aanmaken van een cluster en het beheren van een project.',
  icon: ICONS.compass,
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: 'Fundament',
      lead: 'Het platform waarmee teams zelf Kubernetes-clusters en projecten beheren.',
      bullets: [
        'Geen tickets, geen wachttijd: teams regelen hun eigen infrastructuur.',
        'In deze rondleiding lopen we langs clusters, een cluster aanmaken, en projectbeheer.',
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'dashboard',
      title: 'Het overzicht',
      lead: 'Alle clusters van de organisatie in één blik.',
      bullets: [
        'Elk cluster toont status, regio en het aantal projecten en node pools.',
        'Rechts zie je de echte console, met voorbeelddata.',
      ],
      route: '/',
    },
    {
      id: 'cluster-detail',
      title: 'Een cluster van dichtbij',
      lead: 'Klik door naar een cluster voor status, resourcegebruik en activiteit.',
      bullets: [
        'Resourcegebruik (CPU, geheugen, pods) is direct zichtbaar.',
        'De activiteitenfeed laat zien wat het platform op de achtergrond doet.',
      ],
      route: '/clusters/cl-production',
    },
    {
      id: 'cluster-nodes',
      title: 'Node pools',
      lead: 'Reken- en geheugencapaciteit, opgedeeld in autoscalende node pools.',
      bullets: ['Per pool: machinetype, min/max nodes en gezondheid.'],
      route: '/clusters/cl-production/nodes',
    },
    {
      id: 'cluster-namespaces',
      title: 'Namespaces',
      lead: 'De namespaces die op dit cluster draaien, per project.',
      route: '/clusters/cl-production/namespaces',
      skippable: true,
    },
    {
      id: 'add-cluster',
      title: 'Een nieuw cluster aanmaken',
      lead: 'De wizard vult zichzelf: kijk hoe de clusternaam wordt ingetypt.',
      bullets: [
        'Naam, regio en Kubernetes-versie in stap 1.',
        'Daarna node pools en een samenvatting.',
      ],
      route: '/clusters/add',
      drive: addClusterDrive,
    },
    {
      id: 'projects',
      title: 'Projecten',
      lead: 'Projecten koppelen teams aan namespaces op een cluster.',
      bullets: ['Elk project toont zijn cluster, aantal namespaces en leden.'],
      route: '/projects',
    },
    {
      id: 'project-members',
      title: 'Projectleden',
      lead: 'Teams beheren zelf wie toegang heeft en met welke rol.',
      bullets: ['Rollen: beheerder of viewer, met least privilege als uitgangspunt.'],
      route: '/projects/pr-burgerzaken/members',
    },
    {
      id: 'project-limits',
      title: 'Resource limits',
      lead: 'Standaard resource requests en limits per project.',
      route: '/projects/pr-burgerzaken/limits',
      skippable: true,
    },
    {
      id: 'plugins',
      title: 'Plugins',
      lead: 'Een catalogus van bouwstenen: certificaten, logging, databases, inloggen.',
      bullets: [
        'Wat al draait staat gemarkeerd als geïnstalleerd, met op hoeveel clusters.',
        'Kijk mee: Cert Manager wordt hier op alle clusters geïnstalleerd.',
      ],
      route: '/plugins',
      drive: installPluginDrive,
    },
    closing(
      'Dit was een statische rondleiding met voorbeelddata. De echte console werkt precies zo, met jouw eigen clusters en projecten.',
    ),
  ],
};

// --- Rollen ---------------------------------------------------------------

const developer: Tour = {
  id: 'dev',
  title: 'Daan Hofman · ontwikkelaar',
  lead: 'Van projectoverzicht tot een namespace waar je vandaag op deployt.',
  icon: ICONS.terminal,
  persona: {
    name: 'Daan Hofman',
    role: 'Ontwikkelaar',
    blurb: 'Je bouwt een gemeentedienst en wilt vandaag nog deployen.',
  },
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: 'Daan Hofman',
      lead: 'Ontwikkelaar in het team burgerzaken. Je hebt een namespace nodig, en je hebt hem nu nodig.',
      bullets: [
        'Vroeger: een ticket voor een namespace, en dan wachten op een andere afdeling.',
        'Nu: je regelt het zelf, en je weet precies binnen welke grenzen je werkt.',
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'projects',
      title: 'Waar je aan werkt',
      lead: 'Je projecten, met het cluster waarop ze draaien.',
      bullets: ['Een project bundelt je namespaces, je teamgenoten en je limits.'],
      route: '/projects',
    },
    {
      id: 'project',
      title: 'Het project burgerzaken',
      lead: 'Alles wat je team nodig heeft, op één plek.',
      route: '/projects/pr-burgerzaken',
    },
    {
      id: 'namespaces',
      title: 'Je namespace',
      lead: 'Hier landt je deploy. Geen ticket, geen wachtrij.',
      bullets: ['De namespace bestaat op het cluster zodra het project is aangemaakt.'],
      route: '/projects/pr-burgerzaken/namespaces',
    },
    {
      id: 'limits',
      title: 'Binnen welke grenzen',
      lead: 'Standaard requests en limits, zodat één dienst nooit het cluster opeet.',
      bullets: ['Je ziet de grenzen vooraf, in plaats van ze te ontdekken bij een incident.'],
      route: '/projects/pr-burgerzaken/limits',
      skippable: true,
    },
    {
      id: 'members',
      title: 'Je teamgenoten erbij',
      lead: 'Een nieuwe collega toegang geven doe je zelf, met de rol die past.',
      bullets: ['Beheerder of viewer, met least privilege als uitgangspunt.'],
      route: '/projects/pr-burgerzaken/members',
    },
    {
      id: 'plugins',
      title: 'Wat je niet zelf hoeft te bouwen',
      lead: 'Een database, certificaten, inloggen: het staat in de catalogus.',
      bullets: [
        'Wat op je cluster geïnstalleerd is, kun je in je eigen namespace gebruiken.',
        'Geen eigen Postgres-cluster meer opzetten om te beginnen.',
      ],
      route: '/plugins',
    },
    closing('Je eigen project, je eigen namespace, en je deploy die vandaag draait.'),
  ],
};

const platformEngineer: Tour = {
  id: 'platform',
  title: 'Yara Nijhuis · platform engineer',
  lead: 'Van clusteroverzicht en node pools tot een nieuw cluster in een paar klikken.',
  icon: ICONS.layers,
  persona: {
    name: 'Yara Nijhuis',
    role: 'Platform engineer',
    blurb: 'Je draait de clusters waar alle teams op landen.',
  },
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: 'Yara Nijhuis',
      lead: 'Platform engineer. Je levert de bodem waar de teams van de gemeente op bouwen.',
      bullets: [
        'Je wilt geen namespace-tickets afhandelen, je wilt capaciteit en standaarden bewaken.',
        'Fundament geeft de teams self-service, en jou het overzicht.',
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'dashboard',
      title: 'Alle clusters',
      lead: 'Status, regio, projecten en node pools van de hele organisatie.',
      route: '/',
    },
    {
      id: 'cluster-detail',
      title: 'Capaciteit en activiteit',
      lead: 'CPU, geheugen en pods per cluster, plus wat het platform op de achtergrond doet.',
      bullets: ['De activiteitenfeed laat elke reconciliatie zien, met poging en resultaat.'],
      route: '/clusters/cl-production',
    },
    {
      id: 'nodes',
      title: 'Node pools',
      lead: 'Autoscalende pools met een machinetype en een min/max.',
      bullets: ['Groeit een team, dan groeit de pool mee, binnen de grenzen die jij zet.'],
      route: '/clusters/cl-production/nodes',
    },
    {
      id: 'namespaces',
      title: 'Wie draait er op dit cluster',
      lead: 'De namespaces per project, zodat je weet wat er landt.',
      route: '/clusters/cl-production/namespaces',
      skippable: true,
    },
    {
      id: 'add-cluster',
      title: 'Een nieuw cluster',
      lead: 'Een acceptatiecluster erbij: naam, regio en versie. Kijk hoe de naam wordt ingetypt.',
      bullets: [
        'Daarna node pools en een samenvatting, en het platform reconcilieert de rest.',
        'Elk cluster komt uit dezelfde wizard, dus elk cluster ziet er hetzelfde uit.',
      ],
      route: '/clusters/add',
      drive: addClusterDrive,
    },
    {
      id: 'plugins',
      title: 'De catalogus',
      lead: 'Bouwstenen die je één keer aanzet, en die elk team daarna gewoon kan gebruiken.',
      bullets: [
        'Presets bundelen wat vrijwel elk cluster nodig heeft.',
        'Kijk mee: Cert Manager gaat naar alle clusters tegelijk. De status komt vanzelf op Installed.',
      ],
      route: '/plugins',
      drive: installPluginDrive,
    },
    closing('Teams die zichzelf bedienen, en jij die de bodem bewaakt in plaats van tickets.'),
  ],
};

const securityOfficer: Tour = {
  id: 'security',
  title: 'Ruben de Groot · security officer',
  lead: 'Toegang, least privilege en een audittrail die vanzelf ontstaat.',
  icon: ICONS.shield,
  persona: {
    name: 'Ruben de Groot',
    role: 'Security officer',
    blurb: 'Je bewaakt toegang, least privilege en de audittrail.',
  },
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: 'Ruben de Groot',
      lead: 'Security officer. Je wilt controle, maar je wilt geen poortwachter zijn.',
      bullets: [
        'Self-service klinkt als controleverlies. Dat is het hier niet.',
        'Teams regelen hun toegang zelf, binnen grenzen die vastliggen en zichtbaar zijn.',
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'project-members',
      title: 'Wie mag wat',
      lead: 'Toegang staat per project vast, met een expliciete rol per persoon.',
      bullets: [
        'Beheerder of viewer: geen impliciete rechten, geen gedeelde accounts.',
        'Least privilege is het uitgangspunt, niet een controle achteraf.',
      ],
      route: '/projects/pr-burgerzaken/members',
    },
    {
      id: 'org-members',
      title: 'Wie zit er in de organisatie',
      lead: 'Iedereen met toegang tot het platform, op één lijst.',
      bullets: ['Vertrekt iemand, dan haal je dat op één plek weg.'],
      route: '/organization/members',
    },
    {
      id: 'activity',
      title: 'De audittrail',
      lead: 'Elke wijziging aan een cluster staat in de activiteitenfeed.',
      bullets: [
        'Wat er veranderde, wanneer, en of het lukte.',
        'Je hoeft niemand te vragen wat er gebeurd is.',
      ],
      route: '/clusters/cl-production',
    },
    {
      id: 'org-limits',
      title: 'Grenzen die centraal vastliggen',
      lead: 'Maximale nodes en standaard resource limits gelden voor de hele organisatie.',
      route: '/organization/limits',
      skippable: true,
    },
    {
      id: 'plugin-detail',
      title: 'Wat je binnenhaalt',
      lead: 'Elke plugin heeft een herkomst: een leverancier, een repository en documentatie.',
      bullets: [
        'Teams kiezen uit een catalogus die jij kent, niet uit willekeurige Helm-charts.',
        'Je ziet per plugin waar het vandaan komt voordat het op een cluster landt.',
      ],
      route: '/plugins/pl-cert-manager',
    },
    closing('Controle door grenzen vooraf, in plaats van goedkeuring per aanvraag.'),
  ],
};

const policyMaker: Tour = {
  id: 'beleid',
  title: 'Iris Wolters · CIO',
  lead: 'Waarom een gemeente hier zelf op wil bouwen.',
  icon: ICONS.building,
  persona: {
    name: 'Iris Wolters',
    role: 'CIO',
    blurb: 'Je beslist of de gemeente hierop gaat bouwen.',
  },
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: 'Iris Wolters',
      lead: 'CIO bij een gemeente. Je beslist waar de digitale dienstverlening op draait.',
      bullets: [
        'Je wilt tempo voor je teams, zonder afhankelijk te worden van één leverancier.',
        'En je wilt kunnen uitleggen waar de gegevens van inwoners staan.',
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'why',
      full: true,
      title: 'Wachten is de grootste kostenpost',
      lead: 'Niet de infrastructuur, maar de doorlooptijd eromheen.',
      bullets: [
        'Een namespace die drie weken duurt, kost meer dan de servers die eronder draaien.',
        'Fundament haalt die wachttijd eruit: teams regelen het zelf.',
      ],
    },
    {
      id: 'autonomy',
      full: true,
      title: 'Geen lock-in',
      lead: 'Standaard Kubernetes, open source, en gemeenten die het samen beheren.',
      bullets: [
        'Wat je hier bouwt, draait ook ergens anders.',
        'De keuze om te vertrekken blijft van jou, en dat houdt de samenwerking gezond.',
      ],
    },
    {
      id: 'dashboard',
      title: 'Wat je ervoor terugkrijgt',
      lead: 'Alle clusters van de organisatie, met hun regio, op één scherm.',
      bullets: ['Geen schaduw-IT: je ziet waar wat draait.'],
      route: '/',
    },
    {
      id: 'projects',
      title: 'De teams zelf',
      lead: 'Elk project is een team met een eigen plek op het platform.',
      route: '/projects',
    },
    {
      id: 'plugins',
      title: 'Gebaande paden',
      lead: 'Een gedeelde catalogus, zodat niet elk team zijn eigen wiel uitvindt.',
      bullets: [
        'Wat één gemeente toevoegt, kunnen de andere gebruiken.',
        'Open source, en de herkomst van elke bouwsteen is te controleren.',
      ],
      route: '/plugins',
    },
    closing('Tempo voor je teams, overzicht voor jou, en geen leverancier die de deur dichthoudt.'),
  ],
};

export const TOURS: Record<string, Tour> = {
  [wholeStory.id]: wholeStory,
  [developer.id]: developer,
  [platformEngineer.id]: platformEngineer,
  [securityOfficer.id]: securityOfficer,
  [policyMaker.id]: policyMaker,
};

export const DEFAULT_TOUR_ID = wholeStory.id;

/** Chooser sections: tours without a persona, then the ones told through a role. */
export const STORY_TOURS = Object.values(TOURS).filter((t) => !t.persona);

export const PERSONA_TOURS = Object.values(TOURS).filter((t) => !!t.persona);
