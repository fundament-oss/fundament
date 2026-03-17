import { initFederation } from '@angular-architects/native-federation';

initFederation({})
  // eslint-disable-next-line no-console
  .catch((err) => console.error(err))
  .then(() => import('./bootstrap'))
  // eslint-disable-next-line no-console
  .catch((err) => console.error(err));
