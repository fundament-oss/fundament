export default {
  input: '../authn-api/openapi.yaml',
  output: 'src/generated/authn-api',
  plugins: [
    '@hey-api/typescript',
    '@hey-api/sdk',
    {
      name: '@hey-api/client-fetch',
      bundle: false,
    },
  ],
};
