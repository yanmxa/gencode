/**
 * Planning Module
 *
 * Plan Mode for GenCode - allows the LLM to design implementation
 * approaches before writing code, with read-only exploration tools.
 */

// Types
export type {
  PlanPhase,
  PlanApprovalOption,
  AllowedPrompt,
  PlanModeState,
  PlanFile,
  PlanModeAllowedTool,
  PlanModeBlockedTool,
  ModeType,
  PlanApprovalState,
  PlanModeEvent,
} from './types.js';

export { PLAN_MODE_ALLOWED_TOOLS, PLAN_MODE_BLOCKED_TOOLS } from './types.js';

// State Management
export {
  PlanModeManager,
  getPlanModeManager,
  resetPlanModeManager,
  isPlanModeActive,
  getCurrentMode,
  enterPlanMode,
  exitPlanMode,
  togglePlanMode,
} from './state.js';

// Plan File Utilities
export {
  generatePlanFileName,
  getPlansDir,
  ensurePlansDir,
  createPlanFile,
  readPlanFile,
  writePlanFile,
  listPlanFiles,
  deletePlanFile,
  parseFilesToChange,
  parsePreApprovedPermissions,
  getDisplayPath,
} from './plan-file.js';

// Tools
export { enterPlanModeTool } from './tools/enter-plan-mode.js';
export { exitPlanModeTool } from './tools/exit-plan-mode.js';
