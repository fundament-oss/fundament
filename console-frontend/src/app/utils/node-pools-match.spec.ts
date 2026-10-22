import { nodePoolsMatch } from './node-pools-match';

const live = (name: string, machineType: string, minNodes: number, maxNodes: number) => ({
  name,
  machineType,
  minNodes,
  maxNodes,
});
const desired = (
  name: string,
  machineType: string,
  autoscaleMin: number,
  autoscaleMax: number,
) => ({
  name,
  machineType,
  autoscaleMin,
  autoscaleMax,
});

describe('nodePoolsMatch', () => {
  it('matches the same pools in any order', () => {
    expect(
      nodePoolsMatch(
        [live('a', 'small', 1, 2), live('b', 'large', 0, 1)],
        [desired('b', 'large', 0, 1), desired('a', 'small', 1, 2)],
      ),
    ).toBe(true);
  });

  it('tells a different size, type or set apart', () => {
    expect(nodePoolsMatch([live('a', 'small', 1, 2)], [desired('a', 'small', 1, 3)])).toBe(false);
    expect(nodePoolsMatch([live('a', 'small', 1, 2)], [desired('a', 'large', 1, 2)])).toBe(false);
    expect(nodePoolsMatch([], [desired('a', 'small', 1, 2)])).toBe(false);
    expect(
      nodePoolsMatch(
        [live('a', 'small', 1, 2), live('b', 'small', 1, 2)],
        [desired('a', 'small', 1, 2)],
      ),
    ).toBe(false);
  });
});
