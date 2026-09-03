import { DriveStep, Slide, Tour } from './presentation.model';
import { loc, Localized } from './i18n';
import { PLUGIN_INSTALLS_ENSURE_EVENT, PLUGIN_INSTALLS_RESET_EVENT } from './presentation.tokens';

// Tours are the walkthrough's content. The chooser groups them into "verhalen"
// (no persona) and "word een rol" (with a persona).
//
// Every `route` below must exist in src/app/app.routes.ts *and* be served by the
// demo transport (src/app/demo/mock-transport.ts); cluster/project ids must match
// src/app/demo/fixtures.ts. A route whose RPCs the mock transport doesn't answer
// renders an error pane mid-presentation, so stick to what is already stubbed.
//
// A slide with `embed` instead of `route` frames the marketplace demo, which is a
// second app with the same rules of its own: the path must exist in
// marketplace-frontend/src/app/app.routes.ts and its RPCs must be answered by
// marketplace-frontend/src/app/demo/mock-transport.ts. Plugin ids are shared
// between the two fixture sets on purpose, so 'pl-cert-manager' is the same
// plugin on either side.
//
// Copy is written once per language with `loc(nl, en)`. Structure (ids, routes,
// drive scripts) is shared, so the two locales cannot drift apart and a missing
// translation fails the build rather than the demo.

const CONSOLE_LINK = {
  url: 'https://console.fundament.projects.digilab.network/',
  label: loc(
    'console.fundament.projects.digilab.network',
    'console.fundament.projects.digilab.network',
  ),
};

const KEYS_ASIDE = loc(
  'Gebruik de pijltjestoetsen ← → om door de slides te navigeren. Esc gaat terug naar de keuze.',
  'Use the arrow keys ← → to move through the slides. Esc goes back to the menu.',
);

/**
 * Walks the cluster wizard: types a name into step 1, then carries on through
 * step 2 to the summary. Step 2 (node pools) starts out valid — it seeds one
 * pool with a generated name, the first machine type and a 1–3 autoscale range —
 * so submitting its form as-is lands on the summary. The summary is where the
 * walkthrough stops: the cluster is not actually created.
 * Each page has exactly one `nldd-form`, so the same selector fits both steps.
 */
const addClusterDrive: DriveStep[] = [
  { wait: 900 },
  { set: '#clusterName', value: 'burgerzaken-acc', type: true },
  { wait: 700 },
  { submit: 'nldd-form' },
  { wait: 1400 },
  { submit: 'nldd-form' },
];

/**
 * Puts the installed plugin in place for the slides that show it off. Arriving
 * from the install slide it changes nothing; jumping straight to one of these
 * slides (a `?slide=` link, a restart, stepping backwards) it installs Cert
 * Manager so the sidebar and its screens are not empty.
 */
const installedPluginDrive: DriveStep[] = [{ emit: PLUGIN_INSTALLS_ENSURE_EVENT }];

/**
 * Installs the first plugin in the catalog (cert-manager) on a single cluster.
 *
 * The targets carry `data-tour` attributes rather than being matched on layout
 * classes or DOM position: those belong to the console's presentation and get
 * reshuffled (a grid rewritten to container queries, a `space-y-2` list turned
 * into `flex gap-2`) without anyone realising the walkthrough hung on them.
 * `querySelector` returns the first match, which is the first catalog card —
 * cert-manager, which fixtures.ts deliberately lists first. Inside the modal the
 * first `nldd-checkbox-field` under `install-clusters` is a per-cluster box; the
 * "select all" checkbox sits outside that list, so exactly one cluster is ticked.
 * The reset event first clears any earlier install, so the slide can be replayed.
 */
const installPluginDrive: DriveStep[] = [
  { emit: PLUGIN_INSTALLS_RESET_EVENT },
  { wait: 1400 },
  { click: '[data-tour="plugin-install"]' },
  { wait: 1000 },
  { set: '[data-tour="install-clusters"] nldd-checkbox-field', check: true },
  { wait: 800 },
  { click: '[data-tour="install-confirm"]' },
];

/**
 * Searches the storefront for "cert", which narrows the grid to Cert Manager.
 * The field lives in the marketplace's own header, and its handler reads
 * `event.target.value` where the console's fields read `detail.value`; the drive
 * runner sets the property before dispatching, so one step covers both apps.
 */
const searchMarketplaceDrive: DriveStep[] = [
  { wait: 1200 },
  { set: 'nldd-search-field', value: 'cert', type: true },
];

/** The marketplace listing every tour opens; the id is shared with the console fixtures. */
const CERT_MANAGER_LISTING = '/plugins/pl-cert-manager';

/**
 * The two marketplace slides that the existing tours borrow. They are written
 * once and take the framing that tour needs, so the storefront is introduced in
 * the vocabulary of the story it lands in rather than three times over.
 */
const storefrontSlide = (lead: Localized, bullets?: Localized[]): Slide => ({
  id: 'marketplace',
  title: loc('De marktplaats', 'The marketplace'),
  lead,
  bullets,
  embed: '/',
});

const listingSlide = (lead: Localized, bullets?: Localized[]): Slide => ({
  id: 'marketplace-listing',
  title: loc('Eén plugin van dichtbij', 'One plugin up close'),
  lead,
  bullets,
  embed: CERT_MANAGER_LISTING,
  skippable: true,
});

const closing = (nl: string, en: string): Slide => ({
  id: 'closing',
  kind: 'closing',
  full: true,
  title: loc('Zelf proberen?', 'Try it yourself?'),
  lead: loc(nl, en),
  link: CONSOLE_LINK,
});

// Icons for the chooser cards: SVG path `d`, 24×24 viewBox, stroked.
const ICONS = {
  compass: 'M12 3a9 9 0 100 18 9 9 0 000-18zM15.5 8.5l-2 5-5 2 2-5 5-2z',
  terminal: 'M4 17l6-5-6-5M12 19h8',
  layers: 'M12 3l9 5-9 5-9-5 9-5M3 14l9 5 9-5',
  shield: 'M12 3l7 3v6c0 4.5-3 7.5-7 9-4-1.5-7-4.5-7-9V6l7-3z',
  building: 'M4 21V7l8-4 8 4v14M9 21v-5h6v5M8 11h.01M12 11h.01M16 11h.01',
  storefront: 'M4 9l1.5-5h13L20 9M4 9h16v11H4V9zM4 9a3 3 0 006 0 3 3 0 006 0 3 3 0 004 0',
};

// --- Verhalen -------------------------------------------------------------

const intro: Tour = {
  id: 'intro',
  title: loc('Zo werkt Fundament', 'How Fundament works'),
  lead: loc(
    'Van clusteroverzicht tot een project met een plugin en zijn certificaten.',
    'From the cluster overview to a project with a plugin and its certificates.',
  ),
  icon: ICONS.compass,
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: loc('Fundament', 'Fundament'),
      lead: loc(
        'Het platform waarmee teams zelf Kubernetes-clusters en projecten beheren.',
        'The platform where teams run their own Kubernetes clusters and projects.',
      ),
      bullets: [
        loc(
          'Geen tickets, geen wachttijd: teams regelen hun eigen infrastructuur.',
          'No tickets, no waiting: teams arrange their own infrastructure.',
        ),
        loc(
          'In deze rondleiding lopen we langs clusters, een cluster aanmaken, en projectbeheer.',
          'This tour walks through clusters, creating a cluster, and running a project.',
        ),
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'dashboard',
      title: loc('Het overzicht', 'The overview'),
      lead: loc(
        'Alle clusters van de organisatie in één blik.',
        "Every one of the organisation's clusters at a glance.",
      ),
      bullets: [
        loc(
          'Elk cluster toont status, regio en het aantal projecten en node pools.',
          'Each cluster shows its status, region, and how many projects and node pools it has.',
        ),
        loc(
          'Rechts zie je de echte console, met voorbeelddata.',
          'On the right is the real console, running on sample data.',
        ),
      ],
      route: '/',
    },
    {
      id: 'cluster-detail',
      title: loc('Een cluster van dichtbij', 'A cluster up close'),
      lead: loc(
        'Klik door naar een cluster voor status, resourcegebruik en activiteit.',
        'Open a cluster for its status, resource usage and activity.',
      ),
      bullets: [
        loc(
          'Resourcegebruik (CPU, geheugen, pods) is direct zichtbaar.',
          'Resource usage (CPU, memory, pods) is visible straight away.',
        ),
        loc(
          'De activiteitenfeed laat zien wat het platform op de achtergrond doet.',
          'The activity feed shows what the platform is doing in the background.',
        ),
      ],
      route: '/clusters/cl-production',
    },
    {
      id: 'cluster-nodes',
      title: loc('Node pools', 'Node pools'),
      lead: loc(
        'Reken- en geheugencapaciteit, opgedeeld in autoscalende node pools.',
        'Compute and memory capacity, divided into autoscaling node pools.',
      ),
      bullets: [
        loc(
          'Per pool: machinetype, min/max nodes en gezondheid.',
          'Per pool: machine type, min/max nodes and health.',
        ),
      ],
      route: '/clusters/cl-production/nodes',
    },
    {
      id: 'cluster-namespaces',
      title: loc('Namespaces', 'Namespaces'),
      lead: loc(
        'De namespaces die op dit cluster draaien, per project.',
        'The namespaces running on this cluster, grouped by project.',
      ),
      route: '/clusters/cl-production/namespaces',
      skippable: true,
    },
    {
      id: 'add-cluster',
      title: loc('Een nieuw cluster aanmaken', 'Creating a new cluster'),
      lead: loc(
        'De wizard vult zichzelf: kijk hoe de clusternaam wordt ingetypt.',
        'The wizard fills itself in: watch the cluster name being typed.',
      ),
      bullets: [
        loc(
          'Naam, regio en Kubernetes-versie in stap 1.',
          'Name, region and Kubernetes version in step 1.',
        ),
        loc('Daarna node pools en een samenvatting.', 'Then node pools and a summary.'),
      ],
      route: '/clusters/add',
      drive: addClusterDrive,
    },
    {
      id: 'projects',
      title: loc('Projecten', 'Projects'),
      lead: loc(
        'Projecten koppelen teams aan namespaces op een cluster.',
        'Projects tie teams to namespaces on a cluster.',
      ),
      bullets: [
        loc(
          'Elk project toont zijn cluster, aantal namespaces en leden.',
          'Each project shows its cluster, its namespaces and its members.',
        ),
      ],
      route: '/projects',
    },
    {
      id: 'project-members',
      title: loc('Projectleden', 'Project members'),
      lead: loc(
        'Teams beheren zelf wie toegang heeft en met welke rol.',
        'Teams manage for themselves who has access, and in which role.',
      ),
      bullets: [
        loc(
          'Rollen: beheerder of viewer, met least privilege als uitgangspunt.',
          'Roles: admin or viewer, with least privilege as the starting point.',
        ),
      ],
      route: '/projects/pr-burgerzaken/members',
    },
    {
      id: 'project-limits',
      title: loc('Resource limits', 'Resource limits'),
      lead: loc(
        'Standaard resource requests en limits per project.',
        'Default resource requests and limits, per project.',
      ),
      route: '/projects/pr-burgerzaken/limits',
      skippable: true,
    },
    storefrontSlide(
      loc(
        'Bouwstenen voor je platform staan in een openbare marktplaats.',
        'Building blocks for your platform live in a public marketplace.',
      ),
      [
        loc(
          'Gemaakt door de Fundament-teams, en door gemeenten en leveranciers zelf.',
          'Made by the Fundament teams, and by municipalities and suppliers themselves.',
        ),
      ],
    ),
    listingSlide(
      loc(
        'Cert Manager: wie het maakt, welke versie er staat, en wat het op je cluster mag.',
        'Cert Manager: who makes it, which version is current, and what it may do on your cluster.',
      ),
    ),
    {
      id: 'plugins',
      title: loc('Plugins', 'Plugins'),
      lead: loc(
        'Wat je in de marktplaats vond, installeer je in de console.',
        'What you found in the marketplace, you install in the console.',
      ),
      bullets: [
        loc(
          'Wat al draait staat gemarkeerd als geïnstalleerd, met op hoeveel clusters.',
          'Whatever is already running is marked as installed, and on how many clusters.',
        ),
      ],
      route: '/plugins',
    },
    {
      id: 'plugins-install',
      title: loc('Een plugin installeren', 'Installing a plugin'),
      lead: loc(
        'Installeren doe je zelf, op de clusters die je kiest.',
        'You install it yourself, on the clusters you pick.',
      ),
      bullets: [
        loc(
          'Kijk mee: Cert Manager wordt hier op een cluster geïnstalleerd.',
          'Watch along: Cert Manager is being installed on a cluster here.',
        ),
      ],
      route: '/plugins',
      drive: installPluginDrive,
    },
    {
      id: 'plugin-in-menu',
      title: loc('De plugin staat er meteen in', 'The plugin is there right away'),
      lead: loc(
        'Cert Manager draait nu op het cluster, dus elk project erop krijgt het in zijn menu.',
        'Cert Manager now runs on the cluster, so every project on it gets it in the menu.',
      ),
      bullets: [
        loc(
          'Links in de zijbalk staat Cert Manager, met "Certificates" eronder.',
          'Cert Manager appears in the sidebar on the left, with "Certificates" under it.',
        ),
        loc(
          'De plugin bepaalt zelf welk menu hij meebrengt; het team hoeft niets in te richten.',
          'The plugin brings its own menu along; the team has nothing to set up.',
        ),
      ],
      route: '/projects/pr-burgerzaken',
      drive: installedPluginDrive,
    },
    {
      id: 'plugin-resources',
      title: loc('De plugin gebruiken', 'Using the plugin'),
      lead: loc(
        'De certificaten van dit project, in dezelfde console.',
        "This project's certificates, in the same console.",
      ),
      bullets: [
        loc(
          'Het scherm komt uit de CRD van de plugin: kolommen, velden en detail zijn afgeleid van het schema.',
          "The screen comes from the plugin's CRD: columns, fields and detail are derived from the schema.",
        ),
        loc(
          'Je team beheert zo zijn certificaten zonder kubectl of een apart dashboard.',
          'Your team manages its certificates without kubectl or a separate dashboard.',
        ),
      ],
      // Routes address an *installation*, not a plugin name, so the segment is the
      // installation name the fixtures give cert-manager. The menu links to the CRD
      // by `plural.group`, so the route carries that too.
      route:
        '/projects/pr-burgerzaken/plugin-resources/system--cert-manager/certificates.cert-manager.io',
      drive: installedPluginDrive,
    },
    {
      id: 'plugin-resource-detail',
      title: loc('Eén certificaat van dichtbij', 'One certificate up close'),
      lead: loc(
        'burgerzaken-portaal: waar het certificaat voor geldt, en of het gezond is.',
        'burgerzaken-portaal: what the certificate covers, and whether it is healthy.',
      ),
      bullets: [
        loc(
          'De domeinnaam, de secret waar het certificaat in landt, en wanneer het wordt vernieuwd.',
          'The domain name, the secret the certificate lands in, and when it gets renewed.',
        ),
        loc(
          'Ook dit scherm is niets meer dan het CRD-schema: de console kent cert-manager niet, alleen het schema.',
          'This screen too is nothing but the CRD schema: the console knows nothing of cert-manager, only the schema.',
        ),
      ],
      // Namespaced objects take `?ns=`, but the deck owns the query string; without
      // it the console falls back to matching the object by name, which is what the
      // list links to here anyway.
      route:
        '/projects/pr-burgerzaken/plugin-resources/system--cert-manager/certificates.cert-manager.io/burgerzaken-portaal',
      drive: installedPluginDrive,
    },
    closing(
      'Dit was een statische rondleiding met voorbeelddata. De echte console werkt precies zo, met jouw eigen clusters en projecten.',
      'That was a scripted tour on sample data. The real console works exactly like this, with your own clusters and projects.',
    ),
  ],
};

const marketplace: Tour = {
  id: 'marketplace',
  title: loc('De Plugin Marktplaats', 'The Plugin Marketplace'),
  lead: loc(
    'Van een plugin vinden tot hem installeren, en hoe je er zelf een publiceert.',
    'From finding a plugin to installing it, and how you publish one yourself.',
  ),
  icon: ICONS.storefront,
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: loc('De Plugin Marktplaats', 'The Plugin Marketplace'),
      lead: loc(
        'Eén plek waar bouwstenen voor Fundament te vinden zijn, van welk team ze ook komen.',
        'One place to find building blocks for Fundament, whichever team they come from.',
      ),
      bullets: [
        loc(
          'De marktplaats is de etalage; de console is waar je installeert.',
          'The marketplace is the shop window; the console is where you install.',
        ),
        loc(
          'We lopen beide kanten langs: die van de gebruiker en die van de maker.',
          "We walk both sides: the user's and the maker's.",
        ),
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'storefront',
      title: loc('De etalage', 'The shop window'),
      lead: loc(
        'De catalogus is openbaar. Je hoeft niet ingelogd te zijn om te zien wat er is.',
        'The catalogue is public. You do not need to be signed in to see what is there.',
      ),
      bullets: [
        loc(
          'Gesorteerd op categorie: security, netwerk, observability, data en identity.',
          'Sorted by category: security, networking, observability, data and identity.',
        ),
        loc(
          'Iedereen kan hier kijken: een inkoper, een architect, of een team dat iets zoekt.',
          'Anyone can look here: a buyer, an architect, or a team searching for something.',
        ),
      ],
      embed: '/',
    },
    {
      id: 'search',
      title: loc('Iets zoeken', 'Searching for something'),
      lead: loc(
        'Zoeken gaat over naam, beschrijving, uitgever en tags tegelijk.',
        'Search covers name, description, publisher and tags at once.',
      ),
      bullets: [
        loc(
          'Kijk mee: "cert" laat meteen zien wat er voor certificaten is.',
          'Watch along: "cert" immediately shows what there is for certificates.',
        ),
      ],
      embed: '/',
      drive: searchMarketplaceDrive,
    },
    {
      id: 'listing',
      title: loc('De vermelding', 'The listing'),
      lead: loc(
        'Wie de plugin maakt, welke versie er staat, en waar hij voor bedoeld is.',
        'Who makes the plugin, which version is current, and what it is for.',
      ),
      bullets: [
        loc(
          'De labels zeggen iets over herkomst en ondersteuning: Core, Rijksoverheid, support tijdens kantooruren.',
          'The labels say something about origin and support: Core, Rijksoverheid, support during office hours.',
        ),
        loc(
          'Geen sterren en geen downloadtellers: herkomst is wat telt, niet populariteit.',
          'No stars and no download counters: provenance is what counts, not popularity.',
        ),
      ],
      embed: CERT_MANAGER_LISTING,
    },
    {
      id: 'permissions',
      title: loc('Wat de plugin mag', 'What the plugin may do'),
      lead: loc(
        'Elke vermelding zegt vooraf welke rechten de plugin op je cluster krijgt.',
        'Every listing says up front which rights the plugin gets on your cluster.',
      ),
      bullets: [
        loc(
          'Cert Manager leest en schrijft certificaten, maakt secrets aan, en leest ingresses.',
          'Cert Manager reads and writes certificates, creates secrets, and reads ingresses.',
        ),
        loc(
          'Dat staat er voordat je installeert, niet pas als het al draait.',
          'That is there before you install, not once it is already running.',
        ),
      ],
      embed: CERT_MANAGER_LISTING,
      skippable: true,
    },
    {
      id: 'install',
      title: loc('Installeren gebeurt in de console', 'Installing happens in the console'),
      lead: loc(
        'De marktplaats kent je clusters niet, dus "Install plugin" brengt je naar de console.',
        'The marketplace does not know your clusters, so "Install plugin" takes you to the console.',
      ),
      bullets: [
        loc(
          'Daar is wel bekend wie je bent en welke clusters van jouw organisatie zijn.',
          'There it is known who you are and which clusters belong to your organization.',
        ),
        loc(
          'Kijk mee: Cert Manager wordt hier op een cluster geïnstalleerd.',
          'Watch along: Cert Manager is being installed on a cluster here.',
        ),
      ],
      route: '/plugins',
      drive: installPluginDrive,
    },
    {
      id: 'installed',
      title: loc('En dan staat hij er', 'And then it is there'),
      lead: loc(
        'Cert Manager draait nu op het cluster, dus elk project erop krijgt het in zijn menu.',
        'Cert Manager now runs on the cluster, so every project on it gets it in the menu.',
      ),
      bullets: [
        loc(
          'Van etalage naar draaiende plugin, zonder ticket en zonder tussenpersoon.',
          'From shop window to running plugin, with no ticket and no middleman.',
        ),
      ],
      route: '/projects/pr-burgerzaken',
      drive: installedPluginDrive,
    },
    {
      id: 'my-plugins',
      title: loc('De andere kant: zelf publiceren', 'The other side: publishing yourself'),
      lead: loc(
        'Wie een plugin maakt, ziet zijn eigen inzendingen en de status ervan.',
        'Whoever makes a plugin sees their own submissions and their status.',
      ),
      bullets: [
        loc(
          'Draft, in review, wijzigingen gevraagd, gepubliceerd: elke build heeft zijn eigen status.',
          'Draft, in review, changes requested, published: every build has its own status.',
        ),
        loc(
          'De catalogus groeit dus met wat gemeenten en leveranciers zelf inbrengen.',
          'So the catalogue grows with what municipalities and suppliers bring in themselves.',
        ),
      ],
      embed: '/manage',
    },
    {
      id: 'publishing',
      title: loc('Publiceren met functl', 'Publishing with functl'),
      lead: loc(
        'Een plugin komt binnen via de commandoregel, niet via een formulier.',
        'A plugin arrives over the command line, not through a form.',
      ),
      bullets: [
        loc(
          'Je pusht een build met functl; de vermelding wordt daarvan afgeleid.',
          'You push a build with functl; the listing is derived from it.',
        ),
        loc(
          'Daardoor hoort publiceren bij je pipeline, niet bij een los proces ernaast.',
          'That makes publishing part of your pipeline instead of a separate process beside it.',
        ),
      ],
      embed: '/manage/create',
      skippable: true,
    },
    {
      id: 'review',
      title: loc('Gepusht, beoordeeld, gepubliceerd', 'Pushed, reviewed, published'),
      lead: loc(
        'Tussen jouw push en de etalage zit een centrale review.',
        'Between your push and the shop window sits a central review.',
      ),
      bullets: [
        loc(
          'De statustracker laat zien waar je build staat en wat er nog moet gebeuren.',
          'The status tracker shows where your build is and what still has to happen.',
        ),
        loc(
          'Vraagt de reviewer wijzigingen, dan lees je hier waarom en push je een nieuwe versie.',
          'If the reviewer asks for changes, you read why here and push a new version.',
        ),
      ],
      embed: '/manage/pl-cert-manager',
    },
    closing(
      'De marktplaats en de console zijn twee kanten van dezelfde catalogus: vinden en beoordelen aan de ene kant, installeren en beheren aan de andere.',
      'The marketplace and the console are two sides of one catalogue: finding and judging on one side, installing and running on the other.',
    ),
  ],
};

// --- Rollen ---------------------------------------------------------------

const developer: Tour = {
  id: 'dev',
  title: loc('Daan Hofman · ontwikkelaar', 'Daan Hofman · developer'),
  lead: loc(
    'Van projectoverzicht tot een namespace waar je vandaag op deployt.',
    'From the project overview to a namespace you can deploy to today.',
  ),
  icon: ICONS.terminal,
  persona: {
    name: 'Daan Hofman',
    role: loc('Ontwikkelaar', 'Developer'),
    blurb: loc(
      'Je bouwt een gemeentedienst en wilt vandaag nog deployen.',
      'You are building a municipal service and want to deploy today.',
    ),
  },
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: loc('Daan Hofman', 'Daan Hofman'),
      lead: loc(
        'Ontwikkelaar in het team burgerzaken. Je hebt een namespace nodig, en je hebt hem nu nodig.',
        'Developer on the civil affairs team. You need a namespace, and you need it now.',
      ),
      bullets: [
        loc(
          'Vroeger: een ticket voor een namespace, en dan wachten op een andere afdeling.',
          'It used to be: raise a ticket for a namespace, then wait on another department.',
        ),
        loc(
          'Nu: je regelt het zelf, en je weet precies binnen welke grenzen je werkt.',
          'Now: you arrange it yourself, and you know exactly what limits you work within.',
        ),
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'projects',
      title: loc('Waar je aan werkt', 'What you work on'),
      lead: loc(
        'Je projecten, met het cluster waarop ze draaien.',
        'Your projects, and the cluster each one runs on.',
      ),
      bullets: [
        loc(
          'Een project bundelt je namespaces, je teamgenoten en je limits.',
          'A project bundles your namespaces, your teammates and your limits.',
        ),
      ],
      route: '/projects',
    },
    {
      id: 'project',
      title: loc('Het project burgerzaken', 'The civil affairs project'),
      lead: loc(
        'Alles wat je team nodig heeft, op één plek.',
        'Everything your team needs, in one place.',
      ),
      route: '/projects/pr-burgerzaken',
    },
    {
      id: 'namespaces',
      title: loc('Je namespace', 'Your namespace'),
      lead: loc(
        'Hier landt je deploy. Geen ticket, geen wachtrij.',
        'This is where your deploy lands. No ticket, no queue.',
      ),
      bullets: [
        loc(
          'De namespace bestaat op het cluster zodra het project is aangemaakt.',
          'The namespace exists on the cluster as soon as the project is created.',
        ),
      ],
      route: '/projects/pr-burgerzaken/namespaces',
    },
    {
      id: 'limits',
      title: loc('Binnen welke grenzen', 'The limits you work within'),
      lead: loc(
        'Standaard requests en limits, zodat één dienst nooit het cluster opeet.',
        'Default requests and limits, so no single service can eat the whole cluster.',
      ),
      bullets: [
        loc(
          'Je ziet de grenzen vooraf, in plaats van ze te ontdekken bij een incident.',
          'You see the limits up front, instead of discovering them during an incident.',
        ),
      ],
      route: '/projects/pr-burgerzaken/limits',
      skippable: true,
    },
    {
      id: 'members',
      title: loc('Je teamgenoten erbij', 'Adding your teammates'),
      lead: loc(
        'Een nieuwe collega toegang geven doe je zelf, met de rol die past.',
        'Giving a new colleague access is something you do yourself, in the right role.',
      ),
      bullets: [
        loc(
          'Beheerder of viewer, met least privilege als uitgangspunt.',
          'Admin or viewer, with least privilege as the starting point.',
        ),
      ],
      route: '/projects/pr-burgerzaken/members',
    },
    {
      id: 'plugins',
      title: loc('Wat je niet zelf hoeft te bouwen', "What you don't have to build"),
      lead: loc(
        'Een database, certificaten, inloggen: het staat in de catalogus.',
        'A database, certificates, sign-in: it is all in the catalogue.',
      ),
      bullets: [
        loc(
          'Wat op je cluster geïnstalleerd is, kun je in je eigen namespace gebruiken.',
          'Whatever is installed on your cluster, you can use from your own namespace.',
        ),
        loc(
          'Geen eigen Postgres-cluster meer opzetten om te beginnen.',
          'No more standing up your own Postgres cluster just to get started.',
        ),
      ],
      route: '/plugins',
    },
    closing(
      'Je eigen project, je eigen namespace, en je deploy die vandaag draait.',
      'Your own project, your own namespace, and your deploy running today.',
    ),
  ],
};

const platformEngineer: Tour = {
  id: 'platform',
  title: loc('Yara Nijhuis · platform engineer', 'Yara Nijhuis · platform engineer'),
  lead: loc(
    'Van clusteroverzicht en node pools tot een nieuw cluster in een paar klikken.',
    'From the cluster overview and node pools to a new cluster in a few clicks.',
  ),
  icon: ICONS.layers,
  persona: {
    name: 'Yara Nijhuis',
    role: loc('Platform engineer', 'Platform engineer'),
    blurb: loc(
      'Je draait de clusters waar alle teams op landen.',
      'You run the clusters every team lands on.',
    ),
  },
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: loc('Yara Nijhuis', 'Yara Nijhuis'),
      lead: loc(
        'Platform engineer. Je levert de bodem waar de teams van de gemeente op bouwen.',
        "Platform engineer. You provide the ground the municipality's teams build on.",
      ),
      bullets: [
        loc(
          'Je wilt geen namespace-tickets afhandelen, je wilt capaciteit en standaarden bewaken.',
          'You do not want to work through namespace tickets; you want to watch capacity and standards.',
        ),
        loc(
          'Fundament geeft de teams self-service, en jou het overzicht.',
          'Fundament gives the teams self-service, and gives you the overview.',
        ),
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'dashboard',
      title: loc('Alle clusters', 'Every cluster'),
      lead: loc(
        'Status, regio, projecten en node pools van de hele organisatie.',
        'Status, region, projects and node pools across the whole organisation.',
      ),
      route: '/',
    },
    {
      id: 'cluster-detail',
      title: loc('Capaciteit en activiteit', 'Capacity and activity'),
      lead: loc(
        'CPU, geheugen en pods per cluster, plus wat het platform op de achtergrond doet.',
        'CPU, memory and pods per cluster, plus what the platform is doing in the background.',
      ),
      bullets: [
        loc(
          'De activiteitenfeed laat elke reconciliatie zien, met poging en resultaat.',
          'The activity feed shows every reconciliation, with the attempt and the result.',
        ),
      ],
      route: '/clusters/cl-production',
    },
    {
      id: 'nodes',
      title: loc('Node pools', 'Node pools'),
      lead: loc(
        'Autoscalende pools met een machinetype en een min/max.',
        'Autoscaling pools with a machine type and a min/max.',
      ),
      bullets: [
        loc(
          'Groeit een team, dan groeit de pool mee, binnen de grenzen die jij zet.',
          'When a team grows, the pool grows with it, within the bounds you set.',
        ),
      ],
      route: '/clusters/cl-production/nodes',
    },
    {
      id: 'namespaces',
      title: loc('Wie draait er op dit cluster', 'Who runs on this cluster'),
      lead: loc(
        'De namespaces per project, zodat je weet wat er landt.',
        'The namespaces per project, so you know what lands where.',
      ),
      route: '/clusters/cl-production/namespaces',
      skippable: true,
    },
    {
      id: 'add-cluster',
      title: loc('Een nieuw cluster', 'A new cluster'),
      lead: loc(
        'Een acceptatiecluster erbij: naam, regio en versie. Kijk hoe de naam wordt ingetypt.',
        'One more acceptance cluster: name, region and version. Watch the name being typed.',
      ),
      bullets: [
        loc(
          'Daarna node pools en een samenvatting, en het platform reconcilieert de rest.',
          'Then node pools and a summary, and the platform reconciles the rest.',
        ),
        loc(
          'Elk cluster komt uit dezelfde wizard, dus elk cluster ziet er hetzelfde uit.',
          'Every cluster comes out of the same wizard, so every cluster looks the same.',
        ),
      ],
      route: '/clusters/add',
      drive: addClusterDrive,
    },
    storefrontSlide(
      loc(
        'De catalogus waar je uit put, is geen lijst die jij bijhoudt.',
        'The catalogue you draw from is not a list you maintain yourself.',
      ),
      [
        loc(
          'Wat een ander platformteam publiceert, staat er voor jou ook in.',
          'What another platform team publishes is there for you as well.',
        ),
      ],
    ),
    listingSlide(
      loc(
        'Voor je iets aanzet, zie je wat het op je cluster mag: rechten en capabilities staan in de vermelding.',
        'Before you switch something on, you see what it may do on your cluster: rights and capabilities are in the listing.',
      ),
    ),
    {
      id: 'plugins',
      title: loc('De catalogus', 'The catalogue'),
      lead: loc(
        'Bouwstenen die je één keer aanzet, en die elk team daarna gewoon kan gebruiken.',
        'Building blocks you switch on once, that every team can simply use afterwards.',
      ),
      bullets: [
        loc(
          'Presets bundelen wat vrijwel elk cluster nodig heeft.',
          'Presets bundle what nearly every cluster needs.',
        ),
        loc(
          'Kijk mee: Cert Manager wordt op een cluster geïnstalleerd. De status komt vanzelf op Installed.',
          'Watch along: Cert Manager is installed on a cluster. The status moves to Installed on its own.',
        ),
      ],
      route: '/plugins',
      drive: installPluginDrive,
    },
    closing(
      'Teams die zichzelf bedienen, en jij die de bodem bewaakt in plaats van tickets.',
      'Teams that serve themselves, and you watching the foundation instead of a ticket queue.',
    ),
  ],
};

const securityOfficer: Tour = {
  id: 'security',
  title: loc('Ruben de Groot · security officer', 'Ruben de Groot · security officer'),
  lead: loc(
    'Toegang, least privilege en een audittrail die vanzelf ontstaat.',
    'Access, least privilege, and an audit trail that builds itself.',
  ),
  icon: ICONS.shield,
  persona: {
    name: 'Ruben de Groot',
    role: loc('Security officer', 'Security officer'),
    blurb: loc(
      'Je bewaakt toegang, least privilege en de audittrail.',
      'You watch over access, least privilege and the audit trail.',
    ),
  },
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: loc('Ruben de Groot', 'Ruben de Groot'),
      lead: loc(
        'Security officer. Je wilt controle, maar je wilt geen poortwachter zijn.',
        'Security officer. You want control, but you do not want to be a gatekeeper.',
      ),
      bullets: [
        loc(
          'Self-service klinkt als controleverlies. Dat is het hier niet.',
          'Self-service sounds like losing control. Here, it is not.',
        ),
        loc(
          'Teams regelen hun toegang zelf, binnen grenzen die vastliggen en zichtbaar zijn.',
          'Teams manage their own access, within limits that are fixed and visible.',
        ),
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'project-members',
      title: loc('Wie mag wat', 'Who may do what'),
      lead: loc(
        'Toegang staat per project vast, met een expliciete rol per persoon.',
        'Access is fixed per project, with an explicit role for each person.',
      ),
      bullets: [
        loc(
          'Beheerder of viewer: geen impliciete rechten, geen gedeelde accounts.',
          'Admin or viewer: no implicit rights, no shared accounts.',
        ),
        loc(
          'Least privilege is het uitgangspunt, niet een controle achteraf.',
          'Least privilege is the starting point, not an audit after the fact.',
        ),
      ],
      route: '/projects/pr-burgerzaken/members',
    },
    {
      id: 'org-members',
      title: loc('Wie zit er in de organisatie', 'Who is in the organisation'),
      lead: loc(
        'Iedereen met toegang tot het platform, op één lijst.',
        'Everyone with access to the platform, on a single list.',
      ),
      bullets: [
        loc(
          'Vertrekt iemand, dan haal je dat op één plek weg.',
          'When someone leaves, you revoke it in one place.',
        ),
      ],
      route: '/organization/members',
    },
    {
      id: 'activity',
      title: loc('De audittrail', 'The audit trail'),
      lead: loc(
        'Elke wijziging aan een cluster staat in de activiteitenfeed.',
        'Every change to a cluster shows up in the activity feed.',
      ),
      bullets: [
        loc(
          'Wat er veranderde, wanneer, en of het lukte.',
          'What changed, when, and whether it succeeded.',
        ),
        loc(
          'Je hoeft niemand te vragen wat er gebeurd is.',
          'You never have to ask anyone what happened.',
        ),
      ],
      route: '/clusters/cl-production',
    },
    {
      id: 'org-limits',
      title: loc('Grenzen die centraal vastliggen', 'Limits fixed centrally'),
      lead: loc(
        'Maximale nodes en standaard resource limits gelden voor de hele organisatie.',
        'Maximum nodes and default resource limits apply across the whole organisation.',
      ),
      route: '/organization/limits',
      skippable: true,
    },
    {
      id: 'plugin-detail',
      title: loc('Wat je binnenhaalt', 'What you are bringing in'),
      lead: loc(
        'Elke plugin heeft een herkomst: een leverancier, een repository en documentatie.',
        'Every plugin has a provenance: a supplier, a repository and documentation.',
      ),
      bullets: [
        loc(
          'Teams kiezen uit een catalogus die jij kent, niet uit willekeurige Helm-charts.',
          'Teams pick from a catalogue you know, not from arbitrary Helm charts.',
        ),
        loc(
          'Je ziet per plugin waar het vandaan komt voordat het op een cluster landt.',
          'For each plugin you can see where it came from before it lands on a cluster.',
        ),
      ],
      route: '/plugins/pl-cert-manager',
    },
    closing(
      'Controle door grenzen vooraf, in plaats van goedkeuring per aanvraag.',
      'Control through limits set up front, instead of approval request by request.',
    ),
  ],
};

const policyMaker: Tour = {
  id: 'beleid',
  title: loc('Iris Wolters · CIO', 'Iris Wolters · CIO'),
  lead: loc(
    'Waarom een gemeente hier zelf op wil bouwen.',
    'Why a municipality would want to build on this itself.',
  ),
  icon: ICONS.building,
  persona: {
    name: 'Iris Wolters',
    role: loc('CIO', 'CIO'),
    blurb: loc(
      'Je beslist of de gemeente hierop gaat bouwen.',
      'You decide whether the municipality builds on this.',
    ),
  },
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: loc('Iris Wolters', 'Iris Wolters'),
      lead: loc(
        'CIO bij een gemeente. Je beslist waar de digitale dienstverlening op draait.',
        'CIO at a municipality. You decide what public digital services run on.',
      ),
      bullets: [
        loc(
          'Je wilt tempo voor je teams, zonder afhankelijk te worden van één leverancier.',
          'You want speed for your teams, without becoming dependent on a single supplier.',
        ),
        loc(
          'En je wilt kunnen uitleggen waar de gegevens van inwoners staan.',
          "And you want to be able to explain where residents' data is held.",
        ),
      ],
      aside: KEYS_ASIDE,
    },
    {
      id: 'why',
      full: true,
      title: loc('Wachten is de grootste kostenpost', 'Waiting is the biggest cost'),
      lead: loc(
        'Niet de infrastructuur, maar de doorlooptijd eromheen.',
        'Not the infrastructure, but the lead time around it.',
      ),
      bullets: [
        loc(
          'Een namespace die drie weken duurt, kost meer dan de servers die eronder draaien.',
          'A namespace that takes three weeks costs more than the servers underneath it.',
        ),
        loc(
          'Fundament haalt die wachttijd eruit: teams regelen het zelf.',
          'Fundament takes that wait out: teams arrange it themselves.',
        ),
      ],
    },
    {
      id: 'autonomy',
      full: true,
      title: loc('Geen lock-in', 'No lock-in'),
      lead: loc(
        'Standaard Kubernetes, open source, en gemeenten die het samen beheren.',
        'Standard Kubernetes, open source, and municipalities running it together.',
      ),
      bullets: [
        loc(
          'Wat je hier bouwt, draait ook ergens anders.',
          'What you build here runs somewhere else too.',
        ),
        loc(
          'De keuze om te vertrekken blijft van jou, en dat houdt de samenwerking gezond.',
          'The choice to leave stays yours, and that keeps the partnership healthy.',
        ),
      ],
    },
    {
      id: 'dashboard',
      title: loc('Wat je ervoor terugkrijgt', 'What you get back'),
      lead: loc(
        'Alle clusters van de organisatie, met hun regio, op één scherm.',
        "All of the organisation's clusters, with their regions, on one screen.",
      ),
      bullets: [
        loc('Geen schaduw-IT: je ziet waar wat draait.', 'No shadow IT: you see what runs where.'),
      ],
      route: '/',
    },
    {
      id: 'projects',
      title: loc('De teams zelf', 'The teams themselves'),
      lead: loc(
        'Elk project is een team met een eigen plek op het platform.',
        'Each project is a team with its own place on the platform.',
      ),
      route: '/projects',
    },
    storefrontSlide(
      loc(
        'De gedeelde bouwstenen staan in een openbare marktplaats.',
        'The shared building blocks live in a public marketplace.',
      ),
      [
        loc(
          'Wat één gemeente laat maken, kan een andere daarna gewoon gebruiken.',
          'What one municipality has built can simply be used by another afterwards.',
        ),
      ],
    ),
    listingSlide(
      loc(
        'Elke vermelding zegt wie de maker is, welke ondersteuning erbij hoort, en wat de plugin mag.',
        'Every listing says who the maker is, what support comes with it, and what the plugin may do.',
      ),
      [
        loc(
          'Dat is waar leveranciersonafhankelijkheid concreet wordt: de herkomst is te controleren.',
          'That is where independence from suppliers becomes concrete: the provenance can be checked.',
        ),
      ],
    ),
    {
      id: 'plugins',
      title: loc('Gebaande paden', 'Well-trodden paths'),
      lead: loc(
        'Een gedeelde catalogus, zodat niet elk team zijn eigen wiel uitvindt.',
        'A shared catalogue, so not every team reinvents its own wheel.',
      ),
      bullets: [
        loc(
          'Wat één gemeente toevoegt, kunnen de andere gebruiken.',
          'What one municipality adds, the others can use.',
        ),
        loc(
          'Open source, en de herkomst van elke bouwsteen is te controleren.',
          'Open source, and the provenance of every building block can be checked.',
        ),
      ],
      route: '/plugins',
    },
    closing(
      'Tempo voor je teams, overzicht voor jou, en geen leverancier die de deur dichthoudt.',
      'Speed for your teams, oversight for you, and no supplier holding the door shut.',
    ),
  ],
};

export const TOURS: Record<string, Tour> = {
  [intro.id]: intro,
  [marketplace.id]: marketplace,
  [developer.id]: developer,
  [platformEngineer.id]: platformEngineer,
  [securityOfficer.id]: securityOfficer,
  [policyMaker.id]: policyMaker,
};

export const DEFAULT_TOUR_ID = intro.id;

/** Chooser sections: tours without a persona, then the ones told through a role. */
export const STORY_TOURS = Object.values(TOURS).filter((tour) => !tour.persona);

export const PERSONA_TOURS = Object.values(TOURS).filter((tour) => !!tour.persona);
