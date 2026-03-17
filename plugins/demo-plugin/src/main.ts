import { initFederation } from '@angular-architects/native-federation';

// When running as a standalone dev app, initialize with no remotes.
// When running as a remote loaded by the host, the host controls federation init.
initFederation({})
  .catch((err) => console.error(err))
  .then(() => import('./bootstrap'))
  .catch((err) => console.error(err));
