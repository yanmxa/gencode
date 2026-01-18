/**
 * Component loading reporter
 * Tracks and reports on component loading across the system
 */

export interface ResourceSource {
  level: 'user' | 'project';
  namespace: 'gen' | 'claude';
}

export interface LoadingError {
  file: string;
  error: string;
}

export interface SourceSummary {
  level: 'user' | 'project';
  namespace: 'gen' | 'claude';
  count: number;
}

export interface ComponentLoadingSummary {
  component: string;
  total: number;
  successful: number;
  failed: number;
  sources: SourceSummary[];
  errors: LoadingError[];
  duration: number;
}

interface ComponentState {
  component: string;
  startTime: number;
  successful: number;
  failed: number;
  sources: Map<string, number>; // "user:gen" -> count
  errors: LoadingError[];
}

/**
 * Reporter for tracking component loading progress and results
 */
export class LoadingReporter {
  private states: Map<string, ComponentState> = new Map();
  private summaries: Map<string, ComponentLoadingSummary> = new Map();

  /**
   * Start tracking a component's loading process
   */
  startComponent(component: string): void {
    this.states.set(component, {
      component,
      startTime: Date.now(),
      successful: 0,
      failed: 0,
      sources: new Map(),
      errors: [],
    });
  }

  /**
   * Record a successful resource load
   */
  recordSuccess(component: string, source: ResourceSource): void {
    const state = this.states.get(component);
    if (!state) {
      throw new Error(`Component "${component}" not started. Call startComponent() first.`);
    }

    state.successful++;
    const sourceKey = `${source.level}:${source.namespace}`;
    state.sources.set(sourceKey, (state.sources.get(sourceKey) || 0) + 1);
  }

  /**
   * Record a failed resource load
   */
  recordFailure(component: string, file: string, error: string): void {
    const state = this.states.get(component);
    if (!state) {
      throw new Error(`Component "${component}" not started. Call startComponent() first.`);
    }

    state.failed++;
    state.errors.push({ file, error });
  }

  /**
   * Finish tracking a component and generate summary
   */
  endComponent(component: string): ComponentLoadingSummary {
    const state = this.states.get(component);
    if (!state) {
      throw new Error(`Component "${component}" not started. Call startComponent() first.`);
    }

    const duration = Date.now() - state.startTime;
    const sources: SourceSummary[] = [];

    for (const [key, count] of state.sources.entries()) {
      const [level, namespace] = key.split(':') as ['user' | 'project', 'gen' | 'claude'];
      sources.push({ level, namespace, count });
    }

    const summary: ComponentLoadingSummary = {
      component: state.component,
      total: state.successful + state.failed,
      successful: state.successful,
      failed: state.failed,
      sources,
      errors: state.errors,
      duration,
    };

    this.summaries.set(component, summary);
    return summary;
  }

  /**
   * Get summary for a specific component
   */
  getSummary(component: string): ComponentLoadingSummary | undefined {
    return this.summaries.get(component);
  }

  /**
   * Get all summaries as a plain object
   */
  toJSON(): Record<string, ComponentLoadingSummary> {
    const result: Record<string, ComponentLoadingSummary> = {};
    for (const [component, summary] of this.summaries.entries()) {
      result[component] = summary;
    }
    return result;
  }

  /**
   * Print a formatted summary to console
   */
  printSummary(): void {
    const summaries = Array.from(this.summaries.values());

    if (summaries.length === 0) {
      console.log('No components loaded.');
      return;
    }

    console.log('\n┌─────────────────────────────────────────────────────────┐');
    console.log('│ Component Loading Summary                               │');
    console.log('├─────────────────────────────────────────────────────────┤');

    for (const summary of summaries) {
      const status = summary.failed > 0 ? '⚠' : '✓';
      const line = `│ ${status} ${summary.component.padEnd(12)} ${summary.successful} loaded, ${summary.failed} failed (${summary.duration}ms)`.padEnd(58) + '│';
      console.log(line);

      // Print sources
      for (const source of summary.sources) {
        const sourceLabel = `${source.level}/${source.namespace}`;
        const sourceLine = `│     ${sourceLabel.padEnd(15)} ${source.count}`.padEnd(58) + '│';
        console.log(sourceLine);
      }

      // Print errors if any
      if (summary.errors.length > 0) {
        for (const error of summary.errors.slice(0, 3)) { // Limit to first 3 errors
          const fileName = error.file.split('/').pop() || error.file;
          const errorMsg = error.error.substring(0, 35);
          const errorLine = `│     ✗ ${fileName}: ${errorMsg}`.padEnd(58) + '│';
          console.log(errorLine);
        }
        if (summary.errors.length > 3) {
          const moreLine = `│     ... and ${summary.errors.length - 3} more errors`.padEnd(58) + '│';
          console.log(moreLine);
        }
      }
    }

    console.log('└─────────────────────────────────────────────────────────┘\n');

    // Print overall stats
    const totalComponents = summaries.length;
    const totalLoaded = summaries.reduce((sum, s) => sum + s.successful, 0);
    const totalFailed = summaries.reduce((sum, s) => sum + s.failed, 0);
    const totalDuration = summaries.reduce((sum, s) => sum + s.duration, 0);

    console.log(`Total: ${totalComponents} components, ${totalLoaded} resources loaded, ${totalFailed} failed`);
    console.log(`Total time: ${totalDuration}ms\n`);
  }

  /**
   * Print detailed error report
   */
  printErrors(): void {
    const summaries = Array.from(this.summaries.values());
    const summariesWithErrors = summaries.filter(s => s.errors.length > 0);

    if (summariesWithErrors.length === 0) {
      console.log('No errors during component loading.');
      return;
    }

    console.log('\n┌─────────────────────────────────────────────────────────┐');
    console.log('│ Component Loading Errors                                │');
    console.log('├─────────────────────────────────────────────────────────┤');

    for (const summary of summariesWithErrors) {
      console.log(`│ ${summary.component} (${summary.errors.length} errors)`.padEnd(58) + '│');

      for (const error of summary.errors) {
        console.log(`│   File: ${error.file}`.padEnd(58) + '│');
        console.log(`│   Error: ${error.error}`.padEnd(58) + '│');
        console.log('│'.padEnd(58) + '│');
      }
    }

    console.log('└─────────────────────────────────────────────────────────┘\n');
  }

  /**
   * Check if any component had failures
   */
  hasFailures(): boolean {
    return Array.from(this.summaries.values()).some(s => s.failed > 0);
  }
}
