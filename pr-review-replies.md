# Replies to cubic on PR #651

## 1. `submission-detail.component.spec.ts:163`: assertion searches the whole page

Fixed. The test now scopes the query to the "Submitted metadata" card and asserts that it holds the short description and not the Markdown heading.

## 2. `review-queue.component.ts:171`: filter navigation resets scroll

Fixed. `setFilters` now passes `scroll: 'manual'`, as the storefront's live search does, so neither a keystroke nor a card click scrolls the queue back to the top.

## 3. `submission-detail.component.ts:223`: read `detail.value` from the segmented control

Changed, though the switch did work before: `nldd-segmented-control` sets its own `value` before it dispatches `change` from its host, so `event.target.value` already held the new selection. Reading `detail.value` is the event's documented payload, and it drops the `HTMLInputElement` cast, which was wrong. A new test dispatches the event and checks that the Markdown source shows.

## 4. `submission-detail.component.ts:174`: stale tab title after a failed load

Fixed. Each load resets the title to "Review queue" before fetching, so a failed or missing submission no longer keeps the previous plugin's name in the tab.
