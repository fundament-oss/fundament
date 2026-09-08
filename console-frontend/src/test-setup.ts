// happy-dom does not implement ElementInternals. Several design-system controls
// call attachInternals() in their constructor, so without this any template that
// renders one throws before the test gets to look at anything. The stub carries
// only what those constructors touch — form association and ARIA reflection are
// not what these tests are about.
if (typeof HTMLElement !== 'undefined' && !HTMLElement.prototype.attachInternals) {
  HTMLElement.prototype.attachInternals = function attachInternals(this: HTMLElement) {
    return {
      form: null,
      labels: [],
      shadowRoot: null,
      states: new Set<string>(),
      willValidate: false,
      validity: {},
      validationMessage: '',
      setFormValue: () => {},
      setValidity: () => {},
      checkValidity: () => true,
      reportValidity: () => true,
    } as unknown as ElementInternals;
  };
}

// happy-dom puts localStorage on `window` but leaves the bare global undefined,
// and component field initializers read it directly. An in-memory store per test
// run is enough: nothing here asserts on what was persisted.
if (typeof globalThis.localStorage === 'undefined') {
  const store = new Map<string, string>();
  globalThis.localStorage = {
    get length() {
      return store.size;
    },
    key: (index: number) => [...store.keys()][index] ?? null,
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => {
      store.set(key, String(value));
    },
    removeItem: (key: string) => {
      store.delete(key);
    },
    clear: () => store.clear(),
  } as Storage;
}
