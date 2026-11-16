import pluginRouteScope from './plugin-route-scope';

describe('pluginRouteScope', () => {
  it('reads the cluster route as a cluster', () => {
    expect(pluginRouteScope('c1', { pluginScope: 'cluster' })).toEqual({
      clusterId: 'c1',
      projectId: null,
      backPath: '/clusters/c1',
      menuSection: 'organization',
    });
  });

  it('reads the project route as a project', () => {
    expect(pluginRouteScope('p1', {})).toEqual({
      clusterId: null,
      projectId: 'p1',
      backPath: '/projects/p1',
      menuSection: 'project',
    });
  });
});
