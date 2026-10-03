# Admin Panel Rules

Mandatory rules for every UI under `web/`.

## 1. Always use official shadcn-svelte components

Never hand-write HTML + Tailwind for something that already has a component.

```bash
pnpm dlx shadcn-svelte@latest add <component>   # check first: is it already in src/lib/components/ui?
```

| Need | Component |
| --- | --- |
| Form / input | `Field` + `Input`/`Textarea`/`Select` |
| Buttons | `Button` |
| Data | `Table`, `Card`, `Badge` |
| Navigation | `Sidebar`, `Breadcrumb`, `ToggleGroup` |
| Overlay | `Dialog`, `Sheet`, `AlertDialog` |
| Feedback | `svelte-sonner`, `Alert`, `Skeleton`, `Spinner` |
| Empty state | `Empty` |

Forbidden: styled `div` for callouts/empty states/badges, `<hr>` (use `Separator`),
manual `animate-pulse` (use `Skeleton`), `space-x-*`/`space-y-*` (use
`flex` + `gap-*`).

## 2. Use semantic color tokens

`bg-background`, `text-muted-foreground`, `bg-primary`, … Never raw colors
(`bg-blue-500`) or manual `dark:` overrides.

## 3. Structure

- Admin pages live under `src/routes/(admin)/` → wrapped by sidebar + header.
- Public/standalone pages live outside that group (e.g. `/login`).
- Access the API through the single helper `src/lib/api/client.ts`, never raw
  `fetch` in components.

## 4. Format

- Tabs, single quotes, `import { X } from "$lib/components/ui/x"`.
- Multi-part: `import * as Card from "$lib/components/ui/card"`.
- Single-component: `import { Button } from "$lib/components/ui/button"`.
- Icons: `@lucide/svelte/icons/<name>`, set `data-icon="inline-start|inline-end"`
  inside a `Button` (no size classes).
- Run `pnpm check` before finishing.

Full reference: `.agents/skills/shadcn-svelte/SKILL.md` (+ `rules/`).
