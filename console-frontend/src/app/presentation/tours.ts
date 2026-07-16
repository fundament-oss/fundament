import { Tour } from './presentation.model';

// The first (and currently only) tour. Scope: clusters + projects core.
// Cluster/project ids below must match the demo fixtures (src/app/demo/fixtures.ts).
const clustersProjects: Tour = {
  id: 'clusters-projects',
  title: 'Rondleiding: clusters & projecten',
  lead: 'Van overzicht tot het aanmaken van een cluster en het beheren van een project.',
  slides: [
    {
      id: 'intro',
      kind: 'opening',
      full: true,
      title: 'Fundament Console',
      lead: 'Het platform waarmee teams zelf Kubernetes-clusters en projecten beheren.',
      bullets: [
        'Geen tickets, geen wachttijd: teams regelen hun eigen infrastructuur.',
        'In deze rondleiding lopen we langs clusters, een cluster aanmaken, en projectbeheer.',
      ],
      aside: 'Gebruik de pijltjestoetsen ← → om door de slides te navigeren. Esc sluit de presentatie.',
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
      drive: [
        { wait: 900 },
        { set: '#clusterName', value: 'burgerzaken-acc', type: true },
        { wait: 700 },
        { submit: 'nldd-form' },
      ],
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
      id: 'closing',
      kind: 'closing',
      full: true,
      title: 'Zelf proberen?',
      lead: 'Dit was een statische rondleiding met voorbeelddata. De echte console werkt precies zo, met jouw eigen clusters en projecten.',
      link: {
        url: 'https://console.fundament.projects.digilab.network/',
        label: 'console.fundament.projects.digilab.network',
      },
    },
  ],
};

export const TOURS: Record<string, Tour> = {
  [clustersProjects.id]: clustersProjects,
};

export const DEFAULT_TOUR_ID = clustersProjects.id;
