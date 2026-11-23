import { Injectable } from '@angular/core';
import MetalMetricsService from '../bare-metal/metal-metrics.service';
import metalMetrics from './metal-fixtures';

/**
 * The metal-stack snapshot, for the demo only.
 *
 * The seam is filled once, at construction: the fixtures are a fixed estate.
 * Kept here rather than in the page, so the page does not carry sample data
 * that a real customer would see the day somebody forgets it is there.
 */
@Injectable()
export default class DemoMetalMetricsService extends MetalMetricsService {
  constructor() {
    super();
    this.setSnapshot(metalMetrics);
  }
}
