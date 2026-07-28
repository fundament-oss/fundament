import { create } from '@bufbuild/protobuf';
import UserApiService from './user-api.service';
import { UserSchema } from '../../generated/v1/user_pb';

describe('UserApiService.initials', () => {
  it('takes the first letter of the first two name parts', () => {
    expect(UserApiService.initials('Jan de Vries')).toBe('JD');
    expect(UserApiService.initials('Sara Ahmed')).toBe('SA');
  });

  it('handles a single-word name', () => {
    expect(UserApiService.initials('Alice')).toBe('A');
  });

  it('ignores surrounding and repeated whitespace', () => {
    expect(UserApiService.initials('  Lisa   Chen  ')).toBe('LC');
  });

  it('falls back to ? for an empty name rather than throwing', () => {
    expect(UserApiService.initials('')).toBe('?');
    expect(UserApiService.initials('   ')).toBe('?');
  });
});

describe('UserApiService.color', () => {
  it('is stable for the same id', () => {
    const id = '019dce30-0000-7000-8000-000000000001';

    expect(UserApiService.color(id)).toBe(UserApiService.color(id));
  });

  it('always lands on a real palette entry', () => {
    const ids = Array.from({ length: 50 }, (_, i) => `019dce30-0000-7000-8000-${String(i)}`);

    ids.forEach((id) => expect(UserApiService.color(id)).toMatch(/^bg-[a-z]+-600$/));
  });

  it('spreads across the palette instead of collapsing onto one colour', () => {
    const ids = Array.from({ length: 40 }, (_, i) => `019dce30-0000-7000-8000-00000000${i}`);
    const used = new Set(ids.map((id) => UserApiService.color(id)));

    expect(used.size).toBeGreaterThan(1);
  });
});

describe('UserApiService.mapUser', () => {
  it('derives the avatar fields from the roster entry', () => {
    const user = UserApiService.mapUser(
      create(UserSchema, { id: '019dce30-0000-7000-8000-000000000002', name: 'Bart Jansen' }),
    );

    expect(user.id).toBe('019dce30-0000-7000-8000-000000000002');
    expect(user.name).toBe('Bart Jansen');
    expect(user.initials).toBe('BJ');
    expect(user.color).toBe(UserApiService.color('019dce30-0000-7000-8000-000000000002'));
    // The directory has no presence concept, so everyone is selectable.
    expect(user.available).toBe(true);
  });
});
