/** reloadMembers() came back after the organization changed. The cache then
 *  holds another organization's members, so a caller should leave it alone. */
export default class OrganizationChangedError extends Error {
  constructor() {
    super('The organization changed while its members were being listed');
    this.name = 'OrganizationChangedError';
  }
}
