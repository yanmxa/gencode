# Background Tasks Display - Visual Mockup

## Full Command Output Example

### User Input
```bash
/tasks
```

### Output Display

```
STATUS    ID        DESCRIPTION                    TIME       TYPE
● Running bg-bash-1 npm run test:watch            15m 42s    test
● Running bg-bash-2 docker build production       8m 5s      build
○ Pending bg-bash-3 Waiting for deployment slot   0s         bash
✔ Done    bg-bash-4 npm install dependencies      1m 15s     bash
✔ Done    bg-bash-5 Run linter checks             45s        test
✖ Failed  bg-bash-6 Deploy to production          1m 5s      bash
⊘ Stopped bg-bash-7 Long running test suite       12m 30s    test
```

## Color Legend (Terminal Colors)

| Element | Color Code | Hex | Visual |
|---------|-----------|-----|--------|
| Running (●) | Blue | #60A5FA | 🔵 Bright blue, active |
| Pending (○) | Gray | #64748B | ⚪ Muted gray, waiting |
| Done (✔) | Green | #34D399 | 🟢 Success green |
| Failed (✖) | Red | #F87171 | 🔴 Alert red |
| Stopped (⊘) | Yellow | #FBBF24 | 🟡 Warning yellow |
| Task ID | Indigo | #818CF8 | 🔷 Primary brand color |
| Description | White | #F1F5F9 | ⚪ Main text |
| Time | Gray | #94A3B8 | ⚪ Secondary info |
| Type | Muted | #64748B | ⚪ De-emphasized |

## Column Specifications

| Column | Width | Alignment | Content |
|--------|-------|-----------|---------|
| STATUS | 9 chars | Left | Icon + Label |
| ID | 9 chars | Left | 8-char task ID |
| DESCRIPTION | 33 chars | Left | Task description (truncated) |
| TIME | 10 chars | Right | Elapsed/Duration |
| TYPE | 5+ chars | Left | Task category |

## Status States

### Running State (Active Process)
```
● Running bg-bash-1 npm run test:watch            15m 42s    test
```
- **Icon:** ● (filled circle)
- **Color:** Blue (#60A5FA)
- **Label:** "Running"
- **Meaning:** Task is actively executing
- **Time:** Shows elapsed time, updates in real-time

### Pending State (Queued)
```
○ Pending bg-bash-3 Waiting for deployment slot   0s         bash
```
- **Icon:** ○ (empty circle)
- **Color:** Gray (#64748B)
- **Label:** "Pending"
- **Meaning:** Task is queued, waiting to start
- **Time:** Shows 0s until execution begins

### Completed State (Success)
```
✔ Done    bg-bash-4 npm install dependencies      1m 15s     bash
```
- **Icon:** ✔ (checkmark)
- **Color:** Green (#34D399)
- **Label:** "Done"
- **Meaning:** Task completed successfully
- **Time:** Shows final duration

### Failed State (Error)
```
✖ Failed  bg-bash-6 Deploy to production          1m 5s      bash
```
- **Icon:** ✖ (cross mark)
- **Color:** Red (#F87171)
- **Label:** "Failed"
- **Meaning:** Task exited with error
- **Time:** Shows time before failure

### Cancelled State (User Stopped)
```
⊘ Stopped bg-bash-7 Long running test suite       12m 30s    test
```
- **Icon:** ⊘ (prohibited sign)
- **Color:** Yellow (#FBBF24)
- **Label:** "Stopped"
- **Meaning:** User cancelled the task
- **Time:** Shows time before cancellation

## Task Types

### Auto-Detected Categories

| Type | Detection Pattern | Example |
|------|------------------|---------|
| `test` | Contains "test" | "npm run test:watch" |
| `build` | Contains "build" | "docker build production" |
| `bash` | Default | "Deploy to production" |

Future expansion could include:
- `deploy` - Contains "deploy"
- `install` - Contains "install", "npm i"
- `lint` - Contains "lint", "eslint"
- `custom` - User-defined category

## Edge Cases

### Empty State
```
No tasks found
```

### Overflow (> 10 tasks)
```
STATUS    ID        DESCRIPTION                    TIME       TYPE
● Running bg-bash-1 npm run test:watch            15m 42s    test
● Running bg-bash-2 docker build production       8m 5s      build
✔ Done    bg-bash-3 npm install dependencies      1m 15s     bash
✔ Done    bg-bash-4 Run linter checks             45s        test
✔ Done    bg-bash-5 Build staging environment     2m 30s     build
✔ Done    bg-bash-6 Deploy to staging             1m 10s     bash
✔ Done    bg-bash-7 Run smoke tests               35s        test
✖ Failed  bg-bash-8 Deploy to production          1m 5s      bash
⊘ Stopped bg-bash-9 Long running test suite       12m 30s    test
✔ Done    bg-bash-a Cleanup temporary files       15s        bash

... and 5 more tasks
```

## Filter Examples

### Show Only Running Tasks
```bash
/tasks running
```
Output:
```
STATUS    ID        DESCRIPTION                    TIME       TYPE
● Running bg-bash-1 npm run test:watch            15m 42s    test
● Running bg-bash-2 docker build production       8m 5s      build
○ Pending bg-bash-3 Waiting for deployment slot   0s         bash
```

### Show Only Completed Tasks
```bash
/tasks completed
```
Output:
```
STATUS    ID        DESCRIPTION                    TIME       TYPE
✔ Done    bg-bash-4 npm install dependencies      1m 15s     bash
✔ Done    bg-bash-5 Run linter checks             45s        test
```

### Show Only Failed/Stopped Tasks
```bash
/tasks error
```
Output:
```
STATUS    ID        DESCRIPTION                    TIME       TYPE
✖ Failed  bg-bash-6 Deploy to production          1m 5s      bash
⊘ Stopped bg-bash-7 Long running test suite       12m 30s    test
```

## Responsive Behavior

### Terminal Width: 80+ columns
Full table display as shown above

### Terminal Width: < 80 columns
Could truncate description further:
```
STATUS    ID        DESCRIPTION          TIME       TYPE
● Running bg-bash-1 npm run test:watc   15m 42s    test
```

## Accessibility Features

1. **Multiple Indicators**
   - Icon shape (●○✔✖⊘)
   - Color coding
   - Text label
   - Not relying on any single channel

2. **High Contrast**
   - All colors meet WCAG AA standards
   - Text clearly readable on dark background

3. **Screen Reader Friendly**
   - Text labels provide context
   - Structured table format
   - Logical reading order

4. **Keyboard Navigation**
   - Can scroll through list
   - Tab through interactive elements (future)

## Design Inspiration

Inspired by:
- GitHub CLI (gh) - Clean table layouts
- kubectl - Status indicators
- Docker CLI - Container status display
- Claude Code - Professional terminal UI
- Vercel CLI - Real-time build monitoring

## Future Enhancements Preview

### With Progress Bars
```
STATUS    ID        DESCRIPTION                    TIME       TYPE
● Running bg-bash-1 npm run build     [████████··] 8m 5s      build
                    80% complete
```

### With Interactive Actions
```
STATUS    ID        DESCRIPTION                    TIME       TYPE
● Running bg-bash-1 npm run test      [Press C to cancel]    test
```

### With Grouping
```
═══ RUNNING (2) ═══
● Running bg-bash-1 npm run test                  15m 42s    test
● Running bg-bash-2 docker build                  8m 5s      build

═══ COMPLETED (3) ═══
✔ Done    bg-bash-4 npm install                   1m 15s     bash
✔ Done    bg-bash-5 Run linter                    45s        test
```

---

**Design Philosophy:** Clean, professional, informative, accessible.
