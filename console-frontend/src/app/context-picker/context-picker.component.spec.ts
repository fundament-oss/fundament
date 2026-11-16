import { describe, expect, it } from 'vitest';
import { ClusterStatus } from '../../generated/v1/common_pb';
import { filterTree, type PickerCluster } from './context-picker.component';

const cluster = (name: string, projects: string[]): PickerCluster => ({
  id: name,
  name,
  path: `/clusters/${name}`,
  status: ClusterStatus.UNSPECIFIED,
  projects: projects.map((project) => ({
    id: project,
    name: project,
    path: `/projects/${project}`,
  })),
});

const tree = [
  cluster('production', ['burgerzaken', 'belastingen']),
  cluster('staging', ['burgerzaken-test']),
  cluster('empty', []),
];

describe('filterTree', () => {
  it('leaves the tree alone without a query', () => {
    expect(filterTree(tree, '')).toBe(tree);
    expect(filterTree(tree, '   ')).toBe(tree);
  });

  it('keeps every project of a cluster whose name matches', () => {
    expect(filterTree(tree, 'PROD')).toEqual([tree[0]]);
  });

  it('keeps only the matching projects of any other cluster', () => {
    const result = filterTree(tree, 'burger');
    expect(result.map((c) => c.name)).toEqual(['production', 'staging']);
    expect(result[0].projects.map((p) => p.name)).toEqual(['burgerzaken']);
    expect(result[1].projects.map((p) => p.name)).toEqual(['burgerzaken-test']);
  });

  it('drops a cluster with nothing that matches', () => {
    expect(filterTree(tree, 'nowhere')).toEqual([]);
  });
});
