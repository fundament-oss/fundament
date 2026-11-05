// The components are Dutch first. English for the texts they write themselves,
// such as the line a validation list adds under a field it cannot otherwise
// explain. A `translations` set on an element still wins.
//
// Its own module rather than the entrypoint: the demo build boots from
// main.demo.ts, and both have to set it before the first render.
import { setTranslations } from '@nldd/design-system/translations';
import enUS from '@nldd/design-system/translations/en-US';

setTranslations(enUS);
