import { shareAll, withNativeFederation } from '@angular-architects/native-federation/config.js';

export default withNativeFederation({
  name: 'host',
  shared: {
    ...shareAll({ singleton: true, strictVersion: true, requiredVersion: 'auto' }),
  },
  skip: [
    'rxjs/ajax',
    'rxjs/fetch',
    'rxjs/webSocket',
    'rxjs/testing',
    '@angular-architects/native-federation',
    '@angular-architects/native-federation-runtime',
  ],
});
