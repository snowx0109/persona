# Persona design system

Snapshot commit `738ba824c`, date 2026-11-10. All line numbers refer to that commit and will drift. Verify before relying on one.

**Before you add or change UI or an interaction** on any client, read `docs/design/PRINCIPLES.md` and the parts of this document your change touches, and run the "UI change self-check" at the end. The PR description names the rules it followed and gives a reason for every deviation (`CLAUDE.md`, "Design spec for UI changes").

## Reading route

This table only points the way. Values and rules live in the sections it names, each in one place. Read `docs/design/PRINCIPLES.md` first, then the rows for what you are building.

| You are building | Reuse first (Apple) | Rules |
| --- | --- | --- |
| A list of sessions or pending requests (Agents, Inbox, the Assistant home) | the Agents row styles, `StatusDot`, `AgentStateBadge` | 3.4: "Sessions that need the user", "The work-item row", "List controls"; 5.6 |
| A pending request: a question, a plan, a tool approval | the session's pending area | 4.8 |
| A page title or a session's details | `.pageTitle`, `.detailTitle` | 5.6 |
| A button or a row of buttons | `.themedButton`, `StackingButtonRow`; over a scrolling page's foot `floatingPill` in a `floatingFooter` | 1.2 rule 3, 3.7, 3.12 |
| A chip, a badge or a count | `Chip`, `FilterChip`, `CountBadge` | 3.8; principle 4 for what a number counts |
| A card or another surface | `.surface` | 3.3 |
| A page, a grouped page or a row | `.themedPage()`, `.themedGroupedPage()`, `.themedRow()`, `SettingsForm` | 3.4 |
| A sheet | system sheets with detents | 3.6 |
| A watch-face complication | `WatchFaceTokens`, a snapshot the app writes | 3.13 |
| An empty, loading or failed state | `EmptyStateView`, `Loadable` | 3.9, 3.10 |
| A confirmation or an Undo | the ladder in 4.4 | 4.4 |
| Words: labels, errors, notifications | the string catalogs | 5 |
| A color, font, spacing, radius, size or animation | the tokens in section 2 | 1.2 rule 1, 2 |
| An icon or a brand mark | `ThemedIcon`, `IconTile`, `BrandMark` | 2.8 |
| A field for a secret | `SecretField` | 1.2 rule 4 |
| A key that may be a secret provider's reference | `ProviderKeyField` (iOS, Android), `KeyField` (web) | 1.2 rule 4 |
| A new setting | Settings, showing its current value | principles 2 and 3 |
| Any UI change | the UI change self-check | 1.4 |

## Product design principles

The seven product design principles, with their rules, how to check each one and recent cases, live in `docs/design/PRINCIPLES.md`: 1 defaults first, 2 progressive disclosure, 3 user control, 4 consistency first, 5 model and product are separate, 6 rules are checkable, 7 fast, local-first and stateful. They decide product questions before any rule in this document applies. "Principle N" below refers to that numbering. Apple's Human Interface Guidelines are a source for this document, not its authority: what Persona takes from them is rewritten here as Persona rules (sections 2 to 6), and `docs/design/HIG_AUDIT.md` records where the code still falls short.

## 1. Purpose, scope and sources of truth

This is the project-level design specification. It describes what the code uses today. Apple (iOS first, then watchOS, macOS, visionOS) is the reference implementation. Web and Android are checked against it. Where the code and an older doc disagree, the code wins and the disagreement is listed in section 8.

Scope: tokens, components, interaction patterns, content, dark mode, Dynamic Type and the accessibility baseline. The desktop app reuses the web UI rather than creating another design system; its local setup follows OS defaults (`docs/desktop/desktop.md`). Out of scope: the retired native Mac layout and server-rendered copy beyond the voice rules in section 5. The compliance review against Apple's Human Interface Guidelines is in `docs/design/HIG_AUDIT.md`.

### 1.1 Where each thing lives

| Concern | Apple | Web | Android |
| --- | --- | --- | --- |
| Tokens | `apple/Shared/Design/Theme.swift`, `apple/Shared/Design/ColorSpec.swift`, `apple/Shared/Design/StyleSpec.swift`, `apple/Shared/Design/Palette.swift`, `apple/Shared/StatusPalette.swift` | `web/src/styles.css`, `web/src/ui/look.ts` | `android/core/designsystem/src/main/kotlin/persona/core/designsystem/Tokens.kt` (generated), hand-written siblings in the same folder |
| Components | `apple/Persona/DesignSystem/` | `web/src/ui/atoms.tsx`, `web/src/ui/` | the same Android folder |
| Cross-platform token source | none: `spec/tokens` records the Swift values, and `scripts/assets/test_gen_tokens.py` fails when they drift | none: `web/src/styles.css` is hand-written, and nothing reads the generated CSS | `Tokens.kt` is generated from `spec/tokens` (`spec/tokens/README.md`) by `scripts/assets/gen_tokens.py`; Android CI and the pre-commit hook fail when it is stale |
| Written rules | `docs/apple/ios.md`, `docs/apple/icons.md`, `docs/apple/shell.md`, `docs/apple/sidebar.md` | `docs/web/web.md` | `docs/android/android-design.md` |

Relationship to other docs:

- `CLAUDE.md` ("Design system (iOS)") is the short rule list that agents follow. This document is the longer reference behind it.
- `docs/apple/ios.md` ("Design language") holds the Apple values. Where it differs from this document, this document reflects the code (see DS-01, DS-02, DS-03).
- `docs/apple/icons.md` covers the icon vocabulary and sizing.
- `docs/android/android-design.md` covers Android styles, glass levels and page-writing rules.
- `docs/web/web.md` covers web layout and conventions.
- `docs/SUPER_RULES.md` holds the highest-priority rules, including the product audience foundation and the interaction rules cited in section 4. Changes there need administrator approval.
- `docs/design/DECISIONS.md` is the history of these rules: what was decided, when, and what it replaced. It holds no rules; where it and this document differ, this document wins.

### 1.2 Rules

1. UI code uses only tokens: font level, `Theme.Space`, `Theme.Radius`, `Theme.Size` with `scaledFrame`, `Theme.Motion`, role colors, `Theme.Icon`. A plain `frame` is for lines of 4 pt or less.
2. Never restyle an existing screen directly. A new look is a new `AppStyle` or `ThemeColor`. A change that brings a screen in line with this document is a fix, not a new look; when it changes what a primary screen shows, it is a large change (section 1.4).
3. Button text never wraps (decided 2026-11-10). A button label stays on one line at every text size. When a label would be truncated, work through these in order: (a) shorten the copy, in every language; (b) ask whether a button is the right control here, since long or user-supplied text is content and belongs in a row, a list or the body, and a set of actions that doesn't fit goes into a menu (a decision row's actions are not hidden in a menu: see the exception below); (c) stack the whole button group vertically, primary action first, each label still on one line: at accessibility text sizes (`dynamicTypeSize.isAccessibilitySize`), and at any size where a label would not fit its share of the row. `StackingButtonRow` (3.7) decides that by measured width, not by text size (#6151). Exception, decided in `docs/design/task-cards.md` P1 (#5569): on a narrow width the owner-decision row's text takes the full width and its buttons go below it, the secondary buttons together in one row (wrapping onto the next line whole when they do not fit) and the primary button on its own full-width line last, at every text size; there is no Actions menu. `minimumScaleFactor` is a last resort for small overflows such as long translations, never the fix for a label that is too long. HIG-003 and HIG-012 in `docs/design/HIG_AUDIT.md` are tracked against this rule.
4. Use `SecretField`, not `SecureField`, for tokens and keys. A key a secret provider's reference can stand in for uses
   `ProviderKeyField` (iOS and Android) or `KeyField` (web): its **Use a reference** switch stays with that key (on the
   web, at the end of the key's label row, so in a list of computers' keys each switch sits with its own), and a
   reference is typed in plain text, never masked. Android's extension-secret sheet uses the same explicit switch and
   visible reference field; neither client guesses reference mode from an `op://` prefix
   (`docs/extensions/secret-providers.md` "In the apps").
5. Never write `glassEffect` or `.glass` directly. Go through `.surface` and `.themedButton`.
6. Docs are English only. User-visible strings go through the string catalogs (section 5.5).
7. Apple confirmations use `.accessibleConfirmationDialog` (`apple/Persona/DesignSystem/WindowTextSize.swift`), never SwiftUI's `.confirmationDialog` directly in a page. Through the interface's xxxLarge cap it is the existing confirmation dialog; above the cap it presents an alert, whose labels and button capsules use compatible traits. Keep every action label on one line under rule 3. Set `hasCancel: true` exactly when the existing actions already include Cancel; otherwise the alert adds Cancel once. `ConfirmationDialogRuleTests` rejects direct calls.

### 1.3 Enforcement today

| Test or script | What it guards |
| --- | --- |
| `apple/PersonaTests/TokenRatchetTests.swift` with `apple/PersonaTests/TokenBaseline.json` | Per-file counts of hard-coded padding, HStack spacing, corner radius, hex color, animation, level weight, fixed frame and raw SF Symbol. Counts may only go down. Baseline: 339 hits in 64 files, 46% of them hex colors in terminal, Lark and OS-badge palettes. |
| `apple/PersonaTests/TypeScaleTests.swift` | No system font styles or sizes outside `Theme.Font`. |
| `apple/PersonaTests/IconVocabularyTests.swift` | A registered symbol must be written as its `Theme.Icon` name. |
| `apple/PersonaTests/SheetCloseTests.swift` with `apple/PersonaTests/SheetCloseBaseline.json` | Per-file counts, under `apple/Persona` and outside Mac-only branches, of a presented page's close written by the page instead of `SheetClose` (any item at `.cancellationAction`, or a ✕, Cancel, Close or Done at `.topBarLeading` outside `SheetActions.swift`), and of closes at the trailing edge (✕, a close or cancel role, Cancel, Close, or a Done whose action is one plain statement, at `.confirmationAction`, `.topBarTrailing` or `.primaryAction`; Select's Done and Mark as Done do more and do not count). Counts may only go down. Baseline: none of either since 2026-11-17 (#9653): every presented page closes through `SheetClose`, so any new one fails. |
| `apple/PersonaTests/ConfirmationDialogRuleTests.swift` | Every Swift source under `apple/Persona` uses `.accessibleConfirmationDialog`; the only direct `.confirmationDialog` calls are the shared implementation in `WindowTextSize.swift`. No baseline or per-page exemptions. |
| `apple/PersonaTests/ContinuousMotionTests.swift` | Every unending animation lists where Reduce Motion stops it. |
| `StyleSpecTests` (Apple) | Contrast floors for every style and theme color. |
| `web/src/ui/theme-contrast.test.ts` | AA 4.5:1 for text tokens on seven backgrounds, both schemes. |
| `scripts/app/android_token_baseline.py` | Per-file counts of hard-coded colors, font sizes and dp in every Android module, the design system included (not the generated `Tokens.kt`), and of Material 3 components that pages may not use. Counts may only go down; a wildcard `material3` import outside the design system fails. |
| `scripts/app/android_motion_baseline.py` | Per-file counts of literal animation specs under `android/**/src/main` (`tween` / `spring` / `snap` including empty, typed and positional forms; `keyframes { }` / `keyframes()`; default `AnimatedVisibility` / `Crossfade` / `AnimatedContent` / `animateContentSize` / `animate*AsState` / one-arg `animateTo` / `updateTransition.animate*`; default-spec `fadeIn`/`fadeOut`/`expand*`/`shrink*`/`slide*`/`scale*`). Duration and spring parameters decide token-backed (a token easing alone does not exempt a literal duration). `Tokens.kt` and `PersonaTheme.kt` (adapters) are exempt. Counts may only go down; `--update-baseline` refuses a rise or a new file and checks `version` (#9897). |
| `scripts/app/android_slot_order.py` | An Android design-system component with a composable slot ends with a function-typed parameter (trailing lambdas keep working); the existing exceptions in `android/core/designsystem/tools/slot-order-baseline.json` may only shrink (#4146). |

Not enforced anywhere: hit-area size (except the iOS Agents list's rows, headings and fold controls, `AgentsTouchTargetUITests`, and the iOS Inbox's title buttons, `InboxTitleTargetUITests`), accessibility labels, shadows (inline literals), `.font(.system(size:))` in some Lark and Feishu views, numeric `listRowInsets`, `Spacer(minLength:)` and `lineWidth`.

### 1.4 Who looks at a UI change

Every UI change attaches light and dark screenshots (UI change self-check), the evidence principle 6 asks for. Who looks at them depends on the size of the change, because the mistakes in a large change are the kind that an Agent checking its own work tends to miss.

- **Large**: a person looks at the screenshots before merge, the issue's author or a maintainer. The Agent that made the change does not count. A change is large when it changes a default (a grouping, a style, what a primary screen shows first), a page's structure or navigation, what a primary screen contains, how a pending request looks or works, or a rule in this document.
- **Small**: the Agent that made the change may check its own screenshots. A change is small when it brings a screen back in line with this document, changes copy within section 5, fixes tokens or spacing, or builds a secondary page from existing components.
- The PR says which one it is, and why. When unsure, it is large.

## 2. Design tokens

Colors are written light/dark. iOS sizes are at the default text size.

### 2.1 Color

#### Roles (Apple)

| Token | Value | Source | Web | Android |
| --- | --- | --- | --- | --- |
| `accent` | from the theme color; blue `#007AFF`/`#0A84FF` | `apple/Shared/Design/Theme.swift:104` | `--accent` `#0285ff`/`#3d9aff` (differs; preset "blue" matches Apple) | Glass accent same as Apple |
| `danger` | `#D93A3A`/`#FF5F57` | `apple/Shared/Design/Theme.swift:136` | `--red` `#bd3038`/`#ff9499` | same as Apple |
| `danger` wash | `danger` at 12% | `apple/Shared/Design/Theme.swift` (the failed and interrupted conclusion cards) | `--danger-wash` = `--red` at 12% (#5569); `--red-tint` stays neutral | same as Apple |
| `positive` | `#248A3D`/`#30D158` | `apple/Shared/Design/Theme.swift:137` | `--green` `#107638`/`#6dd697` | same as Apple |
| `caution` / `cautionText` | `#F08A00`/`#FF9F0A`; `#B35400`/`#FF9F0A` | `apple/Shared/Design/Theme.swift:138` | `--orange` `#a64c00`/`#eea66d` (one token) | same as Apple |
| `ai` | `#5B4FD6`/`#9C94FF` | `apple/Shared/Design/Theme.swift:140` | `--ai` same | same |
| `brand` | coral `#D4683F`/`#E8835C`; persona marks only, never tappable things | `apple/Shared/Design/Theme.swift:110` | none | `brand` is the accent (differs) |
| `somedayTint` | `Hue.indigo`, `#5856D6`/`#5E5CE6`, white words at 4.5:1 or better (`StyleSpecTests`); close to `ai` and to Clay's accent, neither of which sits beside it in a row's swipe | `apple/Shared/Design/Theme.swift` (the To-dos row's Someday swipe, iOS, hubs with `todo_view`, from #10465's switch-on) | none yet | none yet |
| `countBadge` | `#636366` both modes, white text at 4.5:1 | `apple/Shared/Design/Theme.swift:153` | accent fill (differs) | same as Apple |
| `onSolid` | white | `apple/Shared/Design/Theme.swift:154` | `text-white` | n/a |
| `Reply.secondaryInk` | `#636366`/`#AEAEB2`; Markdown H4–H6 in replies (#3863), ≥ 4.5:1 on every page, card and the reply box (`StyleSpecTests`), where `.secondary` is about 3.4:1 | `apple/Shared/Design/ReplyTokens.swift` | `--ink-2` `#62656b`/`#a5a8ad` (h4) | none: h4 is `bodyStrong` in the text color (differs) |

`persona` is the persona's own primary (falls back to the accent). `pageBackground`, `card` and `cardStroke` come from the active `StyleSpec`.

#### Agent state colors

Defined once in `apple/Shared/StatusPalette.swift:19-36` and shared by the app, watch and Live Activity. State colors never follow style.

| State | Dot | Text | Notes |
| --- | --- | --- | --- |
| Needs you | system blue `#007AFF`/`#0A84FF` | `#0064C8`/`#409CFF` | blue since #6231 |
| Working | `#E8890C`/`#FFB224` | `#A35A00`/`#FFB224` | amber |
| Unread / awaiting reply / just finished | `#30A46C`/`#4CC38A` | `#1D7A4C`/`#4CC38A` | green; awaiting reply and just finished are a plain dot on the web, not Unread's glow |
| Waiting in the background | working dot at 0.6 | secondary | clock mark; its soft fill the working dot at 0.08 |
| Idle / Offline | secondary at 0.55 / 0.3 | secondary | gray |

Needs you is blue on every client (`docs/design/DECISIONS.md`, 11-11). Android reads these values from the generated tokens (`spec/tokens/color.status.json`); the web still uses amber (`web/src/styles.css:233`), DS-01. Waiting in the background is `status.<mode>.backgroundWaiting` there; Android Glass draws it as iOS does (`PersonaPalette.backgroundWaiting`, picked by `PersonaPalette.status(status, waitsInBackground)` as iOS's `Session.stateTone`), and Brief and Board keep Working's colors for it. It is also a count of its own, on every client and in every style: the quick filters and a heading's state counts (`StateCounts`: a computer's or a space's) leave such an agent out of Working and show it with the clock.

#### Fills, diff, hues, shell neutrals

| Group | Values | Source |
| --- | --- | --- |
| `Fill` | `well` quaternary system fill, `chip` tertiary, `pressed` secondary, `scrim` black 0.45 dark / 0.2 light | `apple/Shared/Design/Theme.swift:229-240` |
| `Diff` | add `#1D7A4C`/`#4CC38A`, del `#C62B33`/`#FF6369`, changed `#8A6A00`/`#E3B341` | `apple/Shared/Design/Theme.swift:170-176` |
| `Hue` (categories only) | blue, orange, green, purple, indigo, pink, gray; with red, amber, yellow, teal, also an extension's 11 named colors (UI version 6, `ExtensionUIStyle.color`), on marks only, never text | `apple/Shared/Design/Theme.swift` |
| `ModeTone` (permission modes, #3719) | asks first: secondary ink; plans: teal `#0E7C86`/`#4FD1C5`; acts within limits (Auto-Edit, Auto): `Hue.purple`; allows everything: `danger`. None is an agent state's color | `apple/Shared/Design/Theme.swift` (`Theme.ModeTone`) |
| `Shell` (same in every style) | composer `#FFFFFF`/`#1F1F21`, stroke `#E3E3E3`/`#3A3A3C`, ink `#0D0D0D`/`#F2F2F2`, segment `#E6E6E6`/`#3A3A3C`, drawer `#F9F9F9`/`#171717` | `apple/Shared/Design/Theme.swift:840-873` |
| `ChatBox` | user `#E3EDFD`/`#1C2F47`, persona `#F0F0F3`/`#2A2A2E` | `apple/Shared/Design/ChatLayoutTokens.swift:18-23` |
| `UsageColor` | input, output, cache write, cache read; color-blind checked; same on web | `apple/Shared/Design/UsageColor.swift:8-15` |
| `Syntax` | eight colors, each 4.5:1 on `codeBackground` | `apple/Shared/Design/Theme.swift:888` |

Android mirrors `Shell`, `ChatBox` and the roles hex for hex (`android/core/designsystem/src/main/kotlin/persona/core/designsystem/HomeTokens.kt`).

#### Theme colors and palettes

`ThemeColor` (`apple/Shared/Design/ColorSpec.swift:96-147`) has style, persona, blue, coral, lavender, teal, lark and extension. Web ports the five presets hex for hex in `web/src/ui/look.ts:21-27`.

| Preset | Accent light/dark | Bubble |
| --- | --- | --- |
| Blue | `#007AFF`/`#0A84FF` | `#DCEAFF`/`#1D3557`, primary text |
| Coral | `#D4683F`/`#CD6E45` | `#B24C25`/`#AF4C28`, white text |
| Lavender | `#7A5AF8`/`#8979EB` | `#6759D3`/`#6756D9`, white text |
| Teal | `#27747F`/`#2A9798` | ink-based |
| Lark | `#3370FF`/`#4C88FF` | `#D1E3FF`/`#133063` |

Readability floors (`apple/Shared/Design/PersonaPalette.swift:27`): white on accent 3.5:1, link 3.0:1, bubble text 4.5:1. A persona or extension color that misses a floor is darkened or lightened automatically. Known exception: Lark dark, 3.35:1 (`spec/tokens/known-failures.json`). Web enforces 4.5:1 instead and uses a darker `--accent-ink` for filled controls.

#### Dark and light

Every role color is a light/dark pair (`Color(light:dark:)`, `apple/Shared/StatusPalette.swift:40-46`). The watch takes the dark value only. The user choice (System, Light, Dark) is applied to windows, not with `preferredColorScheme` (`apple/Persona/DesignSystem/ColorSchemeChoice.swift`). Web uses a class on the root element set before first paint (`web/index.html:31`). See section 6.1.

### 2.2 Typography

Apple levels (`apple/Shared/Design/Theme.swift:286-376`). On iOS each is a Dynamic Type text style.

| Level | iOS definition | iOS pt | Mac pt | Line height (x size) | Web equivalent |
| --- | --- | --- | --- | --- | --- |
| `display` | title, bold (`:334`) | 28 | 24 | 1.2 | none; 20-28 px literals |
| `title` | title2, semibold (`:336`) | 22 | 18 | 1.25 | none |
| `heading` | headline (`:338`) | 17 | 13 | 1.3 | 13-14 px semibold literals |
| `body` | body (`:340`) | 17 | 13 | 1.45 | `--chat-body` 15 px (16 on phones) |
| `secondary` | subheadline (`:342`) | 15 | 12 | 1.4 | `--chat-secondary` 14 px (15) |
| `caption` | footnote (`:344`) | 13 | 11 | 1.35 | 11-12.5 px literals |
| `sectionHeader` | footnote, semibold (`:346`) | 13 | 11 | n/a | 10.5 px uppercase in lists |
| `label` | caption (`:348`) | 12 | 11 | 1.3 | 11-11.5 px literals |
| `code` | footnote, monospaced (`:350`) | 13 | 11 | 1.35 | 12.5 px mono |

Notes:

- The names do not match Apple's text-style names (`caption` is footnote, `label` is Apple's caption). Keep the Persona names in code and use the table above in docs.
- Emphasis tokens (`bodyStrong`, `secondaryStrong`, `captionStrong`, `labelStrong` and so on, `:356-365`) are the only way to change weight. `TypeScaleTests` forbids `.weight(` on a level.
- All non-overridden Glass roles retain their existing `Tokens.Type` mappings from `spec/tokens/type.json`, including shared weights. Long-form reading uses the documented Android-specific values in `spec/tokens/type-glass.json` (`Tokens.TypeGlass`, #5780): body / bodyStrong / voice 16sp and callout / labelSmall 14sp, one point below the corresponding iOS levels. Emphasis is semibold, line height is left to the platform, and Brief / Board are unchanged. `scripts/assets/test_gen_tokens.py` checks the shared levels against `Theme.Font` and `Theme.Level` (DS-12, resolved 11-11).
- Web's shared task-card type tokens in `web/src/styles.css` are `text-card-title` (14.5 px), `text-meta` (13 px), `text-label` (12 px), and `text-submeta` (11.5 px) for secondary meta. They set font size only and leave line height to the context. Elsewhere, about 3,000 `text-[Npx]` uses remain across 20 sizes, some below 11 px.
- Mac uses fixed sizes times `Font.scale`, floor 11 pt.

Dynamic Type behavior:

- The user's Small, Standard or Large setting shifts the system size by -1, 0 or +1 (`apple/Persona/DesignSystem/TextSize.swift:6-36`). Web maps the same ratios (12/13, 1, 14/13) through a build-time rewrite. Android has no in-app setting.
- Shapes scale with `scaledFrame`, capped at 1.6x (`apple/Persona/DesignSystem/TextSize.swift:98-108`). `tapTargetFrame` and `tapTargetHeight` never go below 44 pt.
- The interface's cap is `Theme.interfaceLargestTypeSize` (`.xxxLarge`, section 6.2); reading content lifts it (`.readingContent()`). Named caps below it: `Theme.Shell.topBarLargestTypeSize` is `.xxxLarge` (`apple/Shared/Design/Theme.swift:865`), for the top bars and the Agents search field that rises into their row (#10165); `compactSwitchLargestTypeSize` is `.large`. Other caps are literal at the call site: `xxLarge` for a glyph inside a 44 pt button, `xLarge` for badges (DS-14).
- Documented fixed-size exceptions: Dynamic Island (`apple/Shared/Design/IslandMetrics.swift`), watch face (`apple/Shared/Design/WatchFaceTokens.swift`), Lark rich content that carries sender sizes, and the browser sign-in number (`Theme.Font.signInNumber`, 64 pt rounded bold with fixed-width digits, `apple/Shared/Design/BrowserLoginTokens.swift`, #10293): two digits read off the iPhone to be typed in a browser, already far above the interface's cap at every size.

Web Appearance also offers opt-in interface/code font choices (#10243), independent of text size. Reuse settings rows and native selectors; reveal a single installed-family field only for Custom. Both Default values preserve the original stacks, and resets restore each family independently. User choices precede theme defaults while retaining the existing CJK/symbol fallbacks. No remote fonts are added; terminal size and CJK width correction stay separate. Details: `docs/web/web.md`, “Theme color and text size”.

### 2.3 Spacing

`Theme.Space` (`apple/Shared/Design/Theme.swift:203-223`):

| Token | pt | Meaning |
| --- | --- | --- |
| `flush` | 0 | parts that touch |
| `xxs` | 2 | |
| `xs` | 4 | `lines`: between lines of one row |
| `sm` | 6 | |
| `s` | 8 | `titleToContent` |
| `m` | 12 | |
| `l` | 16 | `block`, `page` (side margin) |
| `xl` | 24 | `section` |
| `xxl` | 32 | |

`sm` (6) is smaller than `s` (8), which reads backwards (DS-17). Web has no spacing tokens (Tailwind 4 px steps; page margin 20/32 px). Android has the same names and values since 11-11 (`spec/tokens/dimension.json`, #4209); its old `xl` (20), which iOS's spacing lacks, became `Tokens.Size.iconXS` where it sized an icon and the gap iOS uses everywhere else (DS-05). Off-grid values in use: card padding default 14 (`CardStyle.padding`, #10233), watch card 10/12, chip vertical 2.5 (DS-09).

Android has a device setting, Settings › Appearance › Layout density (#5564): Standard (the default) draws these values; Compact halves every `Tokens.Space` value (16 → 8, 12 → 6, 8 → 4) and the off-scale paddings written as `N.dp.dense`, takes the style page margin (`pagePadding`) to 0.625 (Glass 16 → 10), rounds corners less (`Tokens.Radius`, `PersonaTheme.shapes` and `N.dp.corner` × 0.6; pills stay pills) and lowers minimum row and button heights written as `N.dp.row` to 0.8 (40 → 32). Text and icons keep their sizes, so a spacing or radius token that sizes something (an icon, a dot, a spinner, a stroke) uses `Tokens.Space.Standard` / `Tokens.Radius.Standard`, which do not follow the setting; new code sizes things with `Tokens.Size` instead, and writes a hard-coded padding as `N.dp.dense`. iOS and web have no density setting yet.

### 2.4 Radius

`Theme.Radius` (`apple/Shared/Design/Theme.swift:181-198`):

| Token | pt | Use | Web | Android |
| --- | --- | --- | --- | --- |
| `thumbnail` | 3 | file thumbnail | literals | missing |
| `badge` | 6 | badges, small marks | chip 6 | 6 |
| `control` | 10 | code blocks, segmented controls | control 8 | 10 |
| `card` | 14 | cards, groups | card 10 | 14 |
| `bubble` | 20 | message bubbles | 14 literal | 20 |
| `panel` | 22 | floating panels | composer 14 | 22 |
| `page` | 55 | page pushed aside as a card | none | 55 |
| `appIcon(side)` | side x 0.2237 | home-screen squircle | mark 8/12 | n/a |

Outside the scale: composer 32 (`Shell.composerRadius`, `apple/Shared/Design/Theme.swift:852`), sidebar rows 6-10, watch card 16, chart 4. Styles differ in color and surface, not in corners (only preview tiles vary). Android varies shapes per style (DS-13). The docs list five radii; the code has seven plus `appIcon` (DS-03).

### 2.5 Sizes

`Theme.Size` (`apple/Shared/Design/Theme.swift:457-570`) always goes through `scaledFrame`.

- Hit target: `tapTarget` 44 (`:507`), drawn through `tapTargetFrame()` (a square for a small control) or `tapTargetHeight()` (a row or a heading that is a button and draws less, as wide as it is; #3753). A line of text that is a link in a row under a card, where a taller row would push the card's buttons under the composer, uses `tapTargetOutset()`: its touch area grows `Space.l` above and below, its layout does not (#5392). A glyph or a small well that must stay where it is drawn (a field's clear ×, a card's corner ×) uses `tapTargetOutset(.all)`: `Space.l` on every side, so a 17 pt glyph is hit as a 49 pt square and a 28 pt well as a 60 pt one (#6577). Neither outset works on a `Menu`'s label: a menu is hit only in the frame its label lays out, so its label takes `tapTargetFrame()` (a list section header's ⋯ grows the header; #6577). An outset is hit only inside what holds the control (a list's cell, a scroll view) and where no later view takes the touch (the page under the link card's header): check it with a tap there, not only with the element's frame. Same number under other names: `searchField`, `switchHeight`, `Watch.minRowHeight`, `Sidebar.row` (iOS), `voiceThreadRoom`. Android uses 48 as its platform floor (`Tokens.MinTouch`).
- Dots 6-10, marks 12-18, icons 20-26, wells 28-38, avatars 40-48. Neighbors differ by 1 to 2 pt (DS-10).
- Named by value: `box50` to `box380` (DS-10). Android's generated tokens carry only the sizes it draws with, under the same names (`spec/tokens/dimension.json`, checked against `Theme.Size`); the space page's cards added `hairline`, `line`, `dotL` and `box150`, the strip of the images a turn looked at `box64` and `box80`, the pill while a chat's model operates a Mac `box280`, the queue strip's number disc `badge`, the to-do page's bottom bar `readable` (#9009).
- The conversation kept in view over the pending cards: `voiceThreadRoom` 44 in a voice call, `cardThreadRoom` 72 over a lone question card (section 4.8, #6277).
- The call screen (#9290, `apple/Shared/Design/VoiceTokens.swift`): `callControl` 72, a round control's disc (Mute, Speaker or Audio, Resume, End); `callAvatar` 160, the assistant's box where the Phone app shows the caller's picture (its face `callAvatarFace` of it, the voice's ring at rest `callAvatarRing`, the hold or muted mark `callAvatarBadge`), `callAvatarShare` the most of the page it takes; `callBarAvatar` 28, its small face in the return bar and the urgent call's header; `callIconTemplate` 40 at `callIconTemplateScale` 3, the system call UI's template icon (`CallIcon`). Its words (#9887 polish, `Theme.Call`): `nameLines` 2, the lines the name under the assistant takes at the regular text sizes, `nameLinesLarge` 3 from `nameLargeFrom` (xLarge) up, each shrinking to `nameShrink` 0.8 before its end is cut; `captionLines` 4, the latest caption's least room in lines, kept while it is empty or short (it takes more where the page has it). Motion: `Theme.Motion.callScreen` the screen sliding up over everything and down, `callScreenOut` how long its window stays for the slide, `callBarRoomCheck` how often the bar looks again at what is over the app.
- The Mac's pointer over its picture in the app's VNC: at least `pointer` 22 tall, about macOS's own size (`VNCPointerArt`, #3075).
- Widths: `readable` 560 (web chat column 880, pages 760), sidebar 340/400/480, drawer `min(0.82 x width, 380)` (Android the same).
- Android regular-window pages keep `Tokens.Size.splitPageMin` (400dp, matching iOS) beside a column. Where the ordinary column and page do not fit, the Assistant sidebar and Agents list overlay a full-width page with the existing scrim; a scrim tap or row pick dismisses it and the existing sidebar control reopens it. Separating hinges retain their existing safe split; compact drawers and wide-window geometry stay unchanged (`docs/android/android-design.md`, section 3.1; #9170).
- Android's bottom bar of a page's actions stops at `Tokens.Size.readable` (560dp, the same logical width as iOS's `Theme.Size.readable`) on a wide window and is centered, in every style: the to-do page's Complete / Reopen and Hand to agent, as iOS's row of pills (#9009, #9008). The page frame alone keeps a bottom bar within `ReadingWidth` + 40dp (800dp), which left each of two buttons about 370dp wide on a tablet or an unfolded screen. The cap grows with the text size, `Tokens.Size.readable` times the font scale (at least 1) up to the page frame's limit, as the tool arguments' label column does with `box120`: a fixed 560dp cut the Japanese 「エージェントに任せる」 from 1.3 times the text size in Glass (1.5 in Brief and Board), where the uncapped bar showed it whole, and this bar has no stacked form as iOS's `StackingButtonRow` gives its row. A window narrower than the cap, a phone upright, keeps the bar's width (`docs/android/android-design.md`, section 3.1).
- Android rich-message table columns: `RichTextTokens.tableColumnMinimum` 160 dp and `tableColumnMaximum` 220 dp include the cell padding and scale with the message's font size; they do not alter a table that fits the upstream renderer unchanged.
- `cardColumn` 330 is the minimum logical column width of an adaptive machine card: iOS pt and Android dp, with the shared token's `px` representing each platform's design unit rather than a physical pixel. `spec/tokens/dimension.json` is the cross-platform record; Android reads the generated `Tokens.Size.cardColumn`.
- Sidebar rows: `Theme.Sidebar` (`apple/Shared/Design/SidebarMetrics.swift`), 44 on iOS; 24/28/32 on Mac; web 32 (touch 44 via a hit-area expander). The web's top-level headings (Pages, Persona chats, Personal Agents, Agents) take `--sidebar-heading`, 24 (touch 44), the height of the tallest control a heading carries, the Agents title row's fold-all button (#10637; iOS's `SidebarHeadingBox`, #10561).
- The working arc (◐, a to-do being worked on in the To-dos list, `WorkingArc` in `apple/Persona/DesignSystem/`; iOS,
  hubs with `todo_view`, from #10465's switch-on): `workingArcSweep` 0.72 of its circle, drawn with `workingArcLine` 2,
  in a `markXS` square that scales with the text.
- Retired native Mac sizes: the former target had a Mac-only token file; that target and its tokens were removed.

### 2.6 Shadow and elevation

Apple has no shadow tokens. Shadows are inline literals:

| Place | Value | Source |
| --- | --- | --- |
| Clay surface | highlight 0.8, r6, (-4,-4); shadow 0.22, r12, (5,8); white stroke 0.45 | `apple/Persona/DesignSystem/Styling.swift:56-59` |
| Island surface | black 0.13, r20, y10 | `apple/Persona/DesignSystem/Styling.swift:66` |
| Soft button | black 0.12 (0.25 tinted), r6, y3 | `apple/Persona/DesignSystem/Styling.swift:183` |
| Composer | black 0.08, r12, y4 | `apple/Persona/DesignSystem/ShellComposerBar.swift:477` |
| Message action bar | black 0.25, r10, y4 | `apple/Persona/Views/Chat/MessageMenu.swift:498` |
| Drawer card (under iPhone Duo's side bars, the sidebar over the page) | black 0.18, r24 | `apple/Persona/Views/Assistant/AssistantSidebar.swift:219` |

The Glass style relies on Liquid Glass and has no shadow. The vendored BeautifulUI package has real elevation tokens that Persona does not read. Web has seven shadow tokens (`web/src/styles.css:68-74`) plus `--shadow-knob` for the switch knob. Android has four literals. See DS-08.

The built-in Mochi face on Android uses `Tokens.Mochi.DarkOffline` for the body gradient on the dark offline disc (#12090). It keeps the online dark outline and uses a lighter shadow, while the eyes, cheek and all other appearance and state combinations keep their colors. See `docs/apple/persona.md`.

### 2.7 Motion

`Theme.Motion` (`apple/Shared/Design/Theme.swift:612-722`).

| Token | Value | Use |
| --- | --- | --- |
| `state` / `standard` | snappy 0.25 s (`:613`) | a state change snaps |
| `layout` | smooth 0.3 s (`:614`) | a layout change glides |
| `expand` | smooth 0.4 s (`:620`) | |
| `reveal` / `conceal` | easeOut 0.28 s / easeIn 0.2 s (`:635`) | `reveal`: what opens in the chat fades in over all its space (`.unfolding`); `conceal`: a menu or an inbox event fading away. What closes in the chat goes at once, its space with it (#3005). `conceal` also fades in a To-dos row the hub re-filed, where it now is (iOS, hubs with `todo_view`, from #10465's switch-on) |
| `drawer(velocity:)` | interpolating spring 0.32 s, no bounce (`:650`) | drawer and pager |
| `springBack` | spring 0.3 s, bounce 0.25 (`:770`) | a finger-dragged message going back to its place (iOS `MessageMenu.swift:470`, `withAnimation(Theme.Motion.springBack)`; Android `swipeToReply`, every style). That call does not read Reduce Motion, on iPhone or iPad, so the spring still runs |
| `todoUndo` | a 4 s window (`Duration`) | a To-dos row ticked, put in Someday or deleted (iOS, hubs with `todo_view`, from #10465's switch-on): its "Done · Undo" toast stays this long and the write waits as long on the device before it goes, so Undo sends nothing (`TodoOutbox`); beside `draftUndo`'s 6 s, the web's window for a dismissed draft. `todoUndoAssistive`, 20 s, while VoiceOver or Switch Control runs: the toast's Undo is out of their reach, so the rows offer it and the write waits that long |
| `alertHandoff` | a 1 s wait (`Duration`) | an alert that follows another (the file browser's queued "already exists" questions) waits before asking: UIKit drops a presentation made while another is closing, and SwiftUI took up to about 0.8 s to close one answered elsewhere (#6113) |
| `keyboard` | interpolating spring (`:662`) | |
| Needs-you breath | 1 s, scale 1.2 | the only attention loop; Android Glass draws the same ring (`JStatusDot`, `JStatusMark`), and keeps the dot still where iOS does (quick filters, a heading's counts, badges, a request's card and panel, sidebar rows: `breathes = false`) |
| `workingArcTurn` | one turn a second, in the compositor | the To-dos list's ◐ on a to-do being worked on (iOS, hubs with `todo_view`, from #10465's switch-on): paused off screen and still with Reduce Motion (`ContinuousMotionTests`) |
| `Theme.Motion.recordingLevelFrame` | a 1/30 s clock | the live recording's level meter only (`RecordingLevelMeter`); fixed-height bars scaled vertically, one still frame with Reduce Motion or Low Power Mode. Reason: the level changes many times a second, and a 30 fps clock read inside the meter redraws only the meter (#4240) |
| Built-in Mochi motion | `personaFrameInterval` (15 fps requested) | On iOS and visionOS, thinking, working and needs-you retain static SwiftUI pieces in `PersonaFaceMotion` and animate only transforms and opacities in the render server. Keyframes come from the shared drawing's actual motion functions, use wall-clock phase and do not restart for equivalent updates. `VisibleWork` remains the visibility owner; ineligible, Reduce Motion, Low Power Mode and held cases use `PersonaFace(moving: false)`, and idle keeps its separate infrequent blink. Mac, widgets and the Live Activity keep their existing renderers |

The composer and attach panel own about 20 more one-off tokens (Telegram-derived; the attach panel itself is a system sheet since #4312). Web: 150 ms state, 220 ms glide, 300 ms collapse, one easing curve, no springs; the reply bar (#5314, cindy's) stays 250 ms after the pointer leaves and fades over 250 ms (`REPLY_BAR_LINGER_MS`, `REPLY_BAR_FADE_MS` in `web/src/ui/motion.ts`). Web sidebar group folds use `--sidebar-fold-open` / `--sidebar-fold-close` (250 / 200 ms) and `--sidebar-fold-open-ease` / `--sidebar-fold-close-ease`, shared by the clipped, fully visible content and its chevron. `spec/tokens/motion.json` carries this shared set (state, layout, expand, reveal, conceal, drawer, springBack, keyboard, the needs-you breath, and the composer's send swap and press, `composerSwap` and `composerPress`: its curve, durations, scales and blur, section 3.1), checked against `Theme.Motion` by `scripts/assets/test_gen_tokens.py`. Android Glass moves with the `state` and `layout` springs (`IosSpring` in `PersonaTheme.kt`: damping ratio `1 - bounce`, stiffness `(2π / duration)²`) and swaps and presses the composer's send as iOS does (`ComposerActionSlot`, `SendButton(pressScales)`); Brief and Board keep their tweens for those style-local springs (DS-11). Every shared entry in `spec/tokens/motion.json` is on generated `Tokens.Motion`. `SharedMotion` exposes the shared call-site specs `expand`, `reveal`, `conceal`, `drawer`, `springBack` and `keyboard` (via `IosMassSpring`); `state` / `layout` live on Glass's `PersonaMotion` / `IosSpring` (Brief and Board keep style tweens); composer send uses `ComposerMotion` / `Tokens.Motion.Composer*`; Needs You breath uses `Tokens.Motion.NeedsYouBreath`. Chat unfolding and the closeout card fold use `reveal` (`CloseoutCard.swift:48`, #9897). Literal `tween` / `spring` / default `AnimatedVisibility` / `Crossfade` counts are ratcheted per file by `scripts/app/android_motion_baseline.py`. Android page stacks share `rememberPageTransitions()` (`PageTransitions.kt`). Glass push and a tapped pop run 350 ms on cubic-bezier(0.28, 1.02, 0.48, 0.96); the page behind travels round(0.3 × width), a black dim of 0.10 × coverage darkens it, and the front card's shadow is black at opacity 0.04, radius 12, offset (0, −3), only while that transition runs. A predictive pop uses the same 350 ms and travel with a linear ease, so the gesture tracks the finger. Those values were measured on the iPhone simulator (Xcode 27.2): a CADisplayLink read the navigation coordinator's 0.35 s transition, `_UIParallaxTransitionCardView`'s shadow and `_UIParallaxDimmingView`, and the cubic was fit to presentation-layer x at the vsync marks. The procedure, including which private views were read, is in `docs/android/android-design.md` (Design system). Brief and Board keep the previous opaque trailing-edge push/pop (300/260 ms, quarter-width parallax, RTL mirrored), their predictive pop uses that same easing, and all three are instant when system animations are off. Beside the sidebar column that page slot is clipped to its inset, so the slide stays in the detail column (`SplitNavigation.swift:140-143`). An upright overlaid Agents list replaces its detail instantly and only the list slides away with Glass's `layout` spring (Brief/Board keep their tweens), copying `SplitNavigation.swift:125-208`. A swiped message springs back with `springBack` in every style (`swipeToReply`, `MessageMenu.swift:470`).

Agents and Inbox rows move only for the tap that changed them (#9600), the same `withAnimation` the iPhone, the open iPhone Duo and iPad already use. On Glass that is the state spring (`Tokens.Motion.State`, `Theme.Motion.standard`) for a section fold (`ChatsView.swift:1168`, a tab at `:2080`), a quiet space (`:2595`, `:3114`, `:3214`, `:3237`), Show all and idle rows (`:3010`, `:3048`, `:3074`), Codex subtasks (`:3289`), Collapse All and Expand All (`AgentsMenus.swift:125`, `:128`), the computer chip (`ChatsView.swift:1646`), pin, unpin, mute and a failed save put back (`PinsStore.swift:131`, `:139`, `:163`), and for Inbox a group's fold (`InboxView.swift:458`), Done's fold (`:1029`), a section fold (`:1127`, `:1130`), bulk Mark as Done (`:634`), Clean up all (`:703`), Clear all (`:1143`), the filter chips (`:1263`, `:1294`) and Clear Filters (`:768`). Changing how Agents are grouped uses the layout spring (`AgentsMenus.swift:405`, `Tokens.Motion.Layout`). Brief and Board use their existing tweens for both. The spec is null unless that tap is in flight. It stays on the rows whose place that tap changed, for the spring (state 250 ms, layout 300 ms): clearing it early draws those rows at their end on the next frame. A row the tap did not move, and a row that arrives afterwards, has no spec, so it jumps. Only a row that was on screen the frame before the tap moves; one that comes back, or that was off screen, fades in where it lands. A group title that is not stuck to the top moves with the rows (`InboxView.swift:456`); the one clamped there stays (`:827`). Inbox keeps an inline title (`InboxView.swift:152-153`); ending a selection inserts no title and does not shift the list (`:632`). Android's large title just appears. The shift it gives every row is not animated; only a row whose order the tap changed springs. The Agents page is throttled to 300 ms. A tap waits for the emission that carries that tap, and a page built before it is ignored. A single swipe (`InboxView.swift:1202`), a section switch (`:1292`), the chat list, the sidebar rows and the search swap are not animated, on either platform. Jump to scrolls (`:924`, `:1099`). `PersonaTheme.reduceMotion`, including the system's animations turned off, makes every one of these instant. iOS passes `nil` under Reduce Motion only for the grouping picker; the other listed `withAnimation(Theme.Motion.standard)` calls do not read it.

A finger on the Assistant history column and on the Agents list, in a regular window, slides that column as on iPad and the open iPhone Duo (`apple/Persona/Views/SplitColumnControl.swift`, `Theme.Split` in `apple/Shared/Design/Theme.swift`). The open drag starts within half a tap target of the leading edge and only while the column is hidden and the detail is at its root (`SplitColumnControl.swift:131-137`, `:160-169`). On Android that open zone is also excluded from system gestures (`RootEdgeExclusion`, same centred 200 dp band as the phone drawer) so system Back does not take it while the detail is at its root; a pushed detail keeps Back and excludes nothing. The close drag starts within that distance of the column's edge and only back toward the edge (`:139-146`); a drag the other way there belongs to Back (`:156`). The line follows the finger one-to-one, clamped to the column (`:254`). The column and the page are still pictures of the rendered frame, taken after the layout they show has been drawn (`snapshotView(afterScreenUpdates: true)`, `SplitColumnControl.swift:216-231`). The overlay is up with the picture of the screen as it is before that layout changes (`:204-216`), then the pictures are stretched into place, so nothing lays out under the finger (`:300-306`, `:330-337`). Blur ramps to 0.8 of the system ultra-thin material across 80 points (`Theme.Split.blurReach`, `Theme.Split.dragBlur`, `:256`). Android draws that with a render effect on API 31 and later, and draws none on older versions; UIKit does not publish the material's radius, so the fraction matches and the radius is a stand-in (`Tokens.Size.iconXS` at a full material). Over the page, black at 0.2 times the line over the width darkens the page picture (`:210`, `:303`, `Theme.Split.overDim`). A 3 point accent guide sits on the line, above the blur (`Theme.Split.guide`, `:305-306`). Faster than 500 points a second, the flick decides; otherwise past half the width stays open, and exactly half stays closed (`:264`). The line then settles on a 0.28 second spring with no bounce, carrying the finger's velocity (`:293`, `Theme.Split.settle`), and the pictures fade over 0.2 seconds (`Theme.Split.fade`). Reduce Motion jumps the line and still fades (`:287-291`). There is no haptic. The open or closed choice is remembered only beside the page, and not during the drag (`apple/Persona/Views/SplitNavigation.swift:491-495`). The button, the scrim and Back keep the animation they already have. Glass, Brief and Board share this finger spring; Brief and Board keep their tweens for that button path. Inbox and Settings do not grow a column drag of their own. The Assistant column's open drag does not begin while a page such as Inbox is pushed, because that edge stays with Back.

On Glass, `JSegmented` and the home Assistant / Agents switch draw one pill that moves to the chosen segment on the `state` spring (`Tokens.Motion.State`, `Theme.Motion.state` in `apple/Shared/Design/Theme.swift`). That is the iPhone shell switch's pill (`apple/Persona/DesignSystem/ShellComponents.swift`, `SwitchMotion` and `matchedGeometryEffect` id `pill`) and the stand-in for a segmented picker's sliding thumb (`Picker` `.segmented`). The pill is in place at once when system animations are off. Brief and Board keep the marks they have. Android has no shell strip, so the pill does not follow a finger the way `PagerPill` does while the strip slides. The label's colour fades on that same spring: iOS animates `foregroundStyle` with `chosen` inside `SwitchMotion` (`ShellComponents.swift` lines 325 and 336; `beautifulSegment` 198–199, `plainSegment` 214–215). Where the weight changes, the two weights crossfade, because SwiftUI crossfades the glyphs when the font changes. An index that is not one of the segments draws no pill.

The Android home switcher's outer capsule uses one Haze surface. A lightly tinted center keeps the labels readable, and incoming light and refraction follow the curved edge. It brightens and grows slightly while pressed. The selected segment is the neutral pill beneath the labels. A lone trailing button uses this material on a circle and draws one third larger while pressed, with a brighter fill and a fixed hit target. Adjacent actions brighten at a fixed size. With Reduce Motion or a less transparent glass level, the switcher's pressed label changes tone and the trailing buttons use their usual ripple while the material stays still.

Android's chat uses the same reveal. `Modifier.unfolding` (`Unfolding.kt`) fades what opens with `Tokens.Motion.Reveal`, copying `Theme.Motion.reveal` (`apple/Shared/Design/Theme.swift:754`) and `.unfolding` (`apple/Persona/Views/Chat/ChatPin.swift:707-726`): the block already has its height, and what closes leaves at once (`ChatPin.swift:686-688`, #3005, #9574). iPhone and regular width use that modifier; no size class skips it. Reduced motion draws the opened block at once (`ChatPin.swift:721-722`).

The phone drawer follows the finger, copying the iPhone (`AssistantDrawer` and `DrawerPan` in `apple/Persona/Views/Assistant/AssistantSidebar.swift`; the release rule is `SwipeRelease.goes` in `apple/Persona/Views/Shell/ShellPager.swift:193-196`). While the finger is down, progress is where the drag started plus the horizontal travel over the drawer width, clamped to 0..1, with animations off (`drag`, lines 100-104). The drag is the drawer's only when it is more horizontal than vertical, and while closed only to the right (`takes`, lines 147-149). It runs only on the Assistant root: a pushed page keeps the drag (`enabled`, line 163; `pushed`, lines 505-509). A leftward drag on a marked swipe row stays the row's, including when that row sits in the vertical list (line 500; the list waits, lines 540-544, and the mark wins). A sideways scroller keeps a rightward drag only while it has room toward its start (`sidewaysScroller`, lines 551-563; refused at line 526 when the offset is past the start, line 558). At that start the drawer takes the drag. On Android the start is the low end of the range, or the high end when the layout is right to left or the scroller sets reverseScrolling, but not both: a rightward drag there has no room. A child that already owns the move, such as a slider, a text selection with its handles showing, or a chart, keeps it whether the drawer is closed or open. A collapsed cursor does not, and neither does a progress indicator. A scroll view under the page waits for the drawer instead (`shouldBeRequiredToFailBy`, lines 540-544), so the list consuming a horizontal move does not count as that child, and a tap handler does not either. A node that is not placed (a list row reused off screen) is not a hit. An Android view keeps a rightward drag only while it can still scroll toward its start, the same room `sidewaysScroller` measures (lines 556-559); a view under the open scrim is not the hit. Semantics does not carry opacity, so only a button in front (a click or the button role) counts as covering that view. Letting go, a flick faster than 300 pt/s goes that way; otherwise progress plus 0.2 s of velocity, divided by the width, opens past one half. Cancelling the touch closes with velocity 0 (`close()`, lines 676-680). Back, or any other open or close while the finger is down, ends the drag and that target wins. Glass settles with `Theme.Motion.drawer` (`apple/Shared/Design/Theme.swift:799-801`), the 0.32 s spring with no bounce, and the finger's speed is the share of the remaining distance per second (`AssistantSidebar.swift:119-121`, `velocity / way`, not the pager's negated share). Brief and Board keep their tweens and ignore that speed. The sidebar lifts by `(1 - progress)` times `Theme.Motion.drawerLift` while it is between closed and open (`AssistantSidebar.swift:69`; `drawerLift` is `Theme.Space.s`, `Theme.swift:805`), and the page hairline fades with progress (line 421). A drag that starts closed and settles open plays a tap haptic (line 110). At a root with the drawer closed, Android excludes a left-edge strip from system gestures (`RootEdgeExclusion`, at most 200 dp tall, centred on the page's vertical middle; width is the left `systemGestures` inset) so a finger there opens the drawer, matching iOS where there is no Back at the Assistant root (`SwipeBack.swift:11-16`). The trailing system-back inset still refuses the drawer; a pushed page keeps the drag off (`DrawerPan`, `AssistantSidebar.swift:505-509`). Three-button navigation has a zero inset and excludes nothing. The iPhone Duo and iPad column edge is a different gesture and is not this drawer. The finger's settle uses that same spring and the finger's speed even when Reduce Motion is on: iOS `drawer(velocity:)` does not read Reduce Motion (`Theme.swift:799-801`). A button, Back, or another write of the open flag keeps the theme's Reduce Motion. Unfolding the phone into the regular-width sidebar snaps the drawer shut. The platform animator scale is what makes the finger's spring instant when animations are off.

Reduce Motion:

- In a view: `accessibilityReduceMotion`. Outside a view: `Theme.Motion.reduced` (`apple/Shared/Design/Theme.swift:639`).
- Tokens do not gate themselves. Each caller must. Loops are covered by `ContinuousMotionTests`. Page transitions, the toast and press scales are not (see the audit).
- Web strips all durations to 1 ms but keeps busy spinners turning (`web/src/styles.css:296-306`). Apple stops loops.
- Haptics are not gated; iOS has no per-app switch.

### 2.8 Icons

- One SF Symbol per meaning, registered in `Theme.Icon` (`apple/Shared/Design/Icons.swift`, 350 entries). Draw through `ThemedIcon` (`apple/Persona/DesignSystem/ThemedIcon.swift`) so the style's icon language applies (weight, outline form, substitutions).
- Some states stay filled in every style (`keepsFill`, `apple/Shared/Design/IconSpec.swift`).
- Beside text, size an icon with a font level. In a box, use `Size.mark*`, `icon*`, `well*` or `avatar*` with `scaledFrame`. Details in `docs/apple/icons.md`.
- Raw symbol names still appear in about 87 baselined places (settings rows, Lark, notification actions, controls).
- Extension catalog artwork uses `ExtensionLogo` with the existing `ExtensionRemoteImage` loader on market rows,
  market details and installed-extension settings. HTTPS artwork is fitted rather than cropped, sized with existing
  `Theme.Size` tokens, decorative beside the extension name, and falls back to `Theme.Icon.plugins` while loading or
  unavailable. `ExtensionImageCache` coalesces URL/pixel-size requests and downscales with ImageIO off the main thread;
  decoded thumbnails have an 8 MiB / 128-entry bound and public requests send no hub credentials or cookies.
  Market and installed lists rank cumulative GHCR package downloads across all versions (not active installations)
  descending, then lowercase name and stable market/item identity; an absent count ranks with zero and is not displayed.
  No new style, setting or navigation is added; at accessibility sizes the market row puts its full name above the
  version and status instead of squeezing all three beside the logo.
- Web draws about 80 stroke glyphs (`web/src/ui/icons.tsx`, 15 px, stroke 1.9). Android uses Phosphor. There is no shared glyph map.
- The web sidebar's page icons are the neutral ink: `--ink-3`, the open page's `--ink` on its fill, never a page's own color. **Colored** sidebar icons are an opt-in (Settings › Appearance › Sidebar icons, #10822) that gives each page a named color; Monochrome stays the default, pixel-identical. The map and its contrast: "Sidebar icon colors" below.
- `pullDown` (#10422, an alias of `expand`'s chevron) follows an action's words that open a menu of how far the action goes (a pull-down, as UIKit draws one): the Inbox group titles' **Mark All Seen ⌄** / **Clear All ⌄** (`docs/assistant/inbox.md` "Group titles"). A value that opens a menu of choices keeps `choices`; the web draws its `chevron`.
- `someday` (#10465, `moon.stars.fill`, drawn as written in every style, being a menu's and a swipe's `Label`; Android's Phosphor `couch`, since `moon-stars` is `quietHours`') is Someday on a to-do, the owner's own "not now": the To-dos row's swipe and menu and the list's ⋯ › Show Someday (iOS, hubs with `todo_view`, from #10465's switch-on). Not `time`, the clock that marks Waiting. Its swipe is `Theme.somedayTint` (the system indigo), beside Done's `positive`; Remove from Someday, its opposite, is `undo`.
- `sideSessions` (#7944) is a session's side sessions on every platform: SF Symbol `rectangle.stack` on Apple, its Phosphor match on Android, Lucide's gallery-vertical-end on the web.
- Catalog package logos (#4780) replace only the generic extension glyph inside its existing leading tile in the market list, market detail, installed list and installed detail. A shared logo component keeps the tile's geometry and fallback glyph. Loaded artwork has a fixed light neutral backing (`--extension-logo-backing` on web, `PersonaPalette.catalogLogoBackground` on Android) so transparent dark marks remain legible in either appearance. Missing, invalid, loading and failed artwork retain the earlier role-tinted generic mark; a changed URL can retry. The image fits without cropping and is decorative beside the package name. Bundled catalog artwork (#5461) is a square PNG inside the versioned extension package; the hub verifies/reads it and returns an authenticated relative `icon_url`. Web uses its selected hub connection to fetch bytes and displays a local Blob URL; Android uses its authenticated hub image loader. Missing, stale or unavailable bundled files keep the same fallback. Legacy absolute HTTPS sources still load anonymously without hub or market credentials. Publisher limits and authenticated image routes are in `docs/extensions/extensions.md` ("Bundled catalog artwork"). Downloads, when supplied, use secondary text (`--extension-meta-size` on web, scaled by the existing text-size transform) and the same GHCR package total across versions on every surface, never an install/user count; unknown counts add no line. This adds package identity to an existing component, not a new style or theme. Other marks, actions and primary screens stay unchanged.

`Theme.Icon.phoneControl` (`iphone.radiowaves.left.and.right`) is the assistant operating an iPhone through a computer:
the phone-control sections and the `iphone_*` tools. Why a separate symbol: `Theme.Icon.phone` (`iphone`) already means a
device, and the phone's own data tools (`phone_*`) keep their icon.

`Theme.Icon.monitor` (`waveform.path.ecg`) is a command watched in the background, each line it prints an event (Claude
Code's `Monitor`). A command run once keeps `Theme.Icon.terminal`.

The call to Persona (#9290): `Theme.Icon.callPersona` (`phone.arrow.up.right`) is **Call Persona** in the main
conversation's + panel, a phone call through the system's call UI, apart from Voice Chat's handset (`voiceChannel`) beside
it, the chat's own voice in the app. On the call screen `callSpeaker` (`speaker.wave.3`, the Phone app's glyph) is the Speaker toggle, not
`voiceSpeaking` (Persona speaking) nor the meeting room's `answersAloud`; `callAudioRoute` (`airplay.audio`, the system's own glyph for it) is Audio, the route
picker; `callMinimise` is `expand`'s chevron, and the voice bar's way back to the call screen, `callExpand`, is `enlarge`.

`Theme.Icon.denied` (`hand.raised.slash`) is a tool call the user, a permission rule or a hook turned down (#4303). It is
drawn in gray at the end of the call's row: the call did not run, so it is not an error and never takes the triangle.

### 2.9 Styles and theme colors

| Style | Surface | Buttons | Canvas | Icons |
| --- | --- | --- | --- | --- |
| `glass` (default) | glass | glass | system | system |
| `clay` | puffy | soft | wash | system |
| `island` | floating | soft | islands | system |
| `lark` | hairline | bordered | neutral | system |
| `beautiful` | glass | glass | neutral | line glyphs |

Source: `apple/Shared/Design/Theme.swift:11-39`, `apple/Shared/Design/StyleSpec.swift`. Style and theme color are independent axes; `StyleSpecTests` checks every pair. New code never switches on `AppStyle`. It reads the composed spec. Web has one look (the Beautiful style). Android has Glass, Brief and Board, so only Glass is common to both platforms.

**Before iOS 26** (the iPhone and iPad app runs from iOS 18.6, #6730) there is no Liquid Glass. The `glass` surface draws
visionOS's material instead (`.regularMaterial`, the tint washed over it), `glass` buttons draw the `bordered` style's,
and `GlassGroup` stands in for `GlassEffectContainer`. Other iOS 26-only modifiers go through
`apple/Persona/DesignSystem/SystemCompat.swift` (`navigationSubtitleWhenSupported`, `listRowVerticalInsets`,
`scrollEdgeEffectHiddenWhenSupported`, `safeAreaBarWhenSupported`, …): on iOS 26 each is the system modifier, so every
screen there is unchanged; before it, the nearest older behavior or nothing. Toolbar items that need iOS 26
(`ToolbarSpacer`, `sharedBackgroundVisibility`) are chosen on the view: `ToolbarContentBuilder` cannot take
`if #available` (`docs/lessons/ios.md`, "iOS 18.6 floor").

On the retired native Mac, a style could also bring the window's chrome (`StyleSpec.MacChrome`), read through the former Mac-only tokens and `Theme.monogram(_:)`: the sections column, the side column beside a list, how the
shell's lists showed the selected row and the row under the pointer, and the discs of names without a picture. Every style
but Lark kept the system's (`.system`), so those Mac windows were unchanged; the iPhone, iPad and watch draw none of it.
Lark's follows the Feishu desktop window (#5598):

| Part | Light / dark | Token |
| --- | --- | --- |
| Sections column | `#E4EAF6` / `#1C1D21` | `Theme.macSidebar` |
| Selected section | a raised block `#FFFFFF` / `#2C2E34`, its icon in the accent, the title semibold | `Theme.Fill.macSidebarSelected`, `Theme.macSelectedIcon`, `Theme.macSelectedWeight` |
| Side column (Inbox's filters) | `#EDF1F7` / `#18191C` | `Theme.macSideColumn` |
| Other selected rows | the accent at 12 %, the icon in the accent | `Theme.Fill.macSelected` |
| Row under the pointer | `#1F2329` / `#FFFFFF` at 6 % | `Theme.Fill.macHover` |
| Names without a picture | one of nine hues picked by the name (the same on every launch), white letter ≥ 3.6:1 | `Theme.monogram(_:)` |

`StyleSpecTests.theMacChromeReads` checks it in the style's own colors: the accent icon ≥ 3:1 on every selected shade,
the row's words ≥ 4.5:1, the white letter ≥ 3:1 on every hue.

## 3. Components

For every component: use the design-system one, not a lookalike. Counts are approximate uses at the snapshot. "Platforms" lists web and Android equivalents.

### 3.1 Composer

The web model panel's current Codex row keeps the session's recorded thinking effort, independently of whether it can
change the model. When only effort can change, tapping the row opens its existing setup flyout instead of presenting
a grey, inert choice. Unsupported effort has a visible reason (offline, read-only, parent-controlled, terminal-only,
or unsupported by the model); supported changes say they apply to the next turn. These details belong to the Agent's
on-demand model panel, not the everyday Assistant home (#6887).

- Use: the one message input for Assistant and Agents chats on iPhone, iPad and Mac.
- Anatomy: collapsed line `[+] field [mic] [voice/stop]`; expanded card with the field on top and a row `[+] ... [model/effort] [mic] [send]`. Both shapes share one text field so focus and IME text survive the change, and one +, mic, voice and stop (`ComposerBoxLayout`, #7278): they sit at the box's bottom corners and move with it between the shapes, in the box's own motion, rather than one copy leaving as another arrives. Only what the card alone has (Hide Keyboard, model, send) fades in and out where it stands.
- Tokens: radius 32, min field height 44, iOS grows from one through six lines then scrolls (`Theme.Shell.composerPhoneMaximumLines`); Mac retains its eight-line cap. Fill and stroke come from `Shell`.
- Android Glass uses the opaque Shell card fill, stroke and shadow in the Assistant home and compact Agent detail composers. The compact Agent composer has a 48dp idle form and a 100dp one-line focused form at standard text size. The focused toolbar is inset 6dp from the sides and 4dp from the bottom. Hide Keyboard uses a keyboard with a downward chevron. The touch targets are 48dp, and empty accessories take up no height. The model pill appears when the toolbar has room for its touch target. On a narrow phone, dismissing the keyboard can free that room while keeping the draft. Expand keeps the same draft, and the add menu keeps the mention action alongside attachments.
- Draft scrolling (iOS, #5650): typing and cursor movement keep the caret visible; unrelated layout updates only correct an offset past the content end. A manual scroll wins over queued caret work and stays put until new input or cursor movement.
- Full-page editor (iOS, #5650): a dedicated control row holds Collapse at its leading end (the page's way out, #9653) and Hide Keyboard at its trailing end, in the composer's neutral icon ink, with 44 pt touch targets. Quotes and editable text start below that row, never behind the controls. This serves anyone composing and reviewing a long message; it requires no new setting.
- States: expanded when focused, non-empty or holding attachments; send hidden when empty and replaced by voice or stop; send gate button when the main conversation is not ready.
- Send and stop swap (iOS `ShellComposerBar` and `ComposerButtons.swift`, `Theme.Motion.composerSwap`; Android Glass, #4210 T3): the round button changes with the state, and the change moves. Send grows from a scale of 0.001 at the trailing edge while a 4 pt blur clears, and fades in over 0.2 s; stop (and voice) shrink to 0.2 from their middle as they fade, and grow from that coming back; send leaving does the same as coming, the other way. Scale and blur follow the curve (0.38, 0.7, 0.125, 1) over 0.4 s, the fade eases in and out over 0.2 s, and what stands beside the slot glides with the `state` spring. A button on its way out takes no tap and is not read out. Send shrinks to 0.75 under a finger, 0.4 s easing out, and comes back over 0.25 s on the swap's curve; stop and the other composers' send buttons keep the plain press, as on iOS. With Reduce Motion (Android: the system's animations off) nothing moves: the swap is instant and send does not shrink. iOS still keeps the scale and blur under Reduce Motion (HIG-044); Android does not copy that. Android skips the blur on Android 11 (no render effects before 12) and keeps Brief's and Board's instant swap (decision 8 of #4194). Android has no Voice Chat button in the slot yet, so only send and stop swap. The values are `composerSwap` and `composerPress` in `spec/tokens/motion.json`.
- Model menu (`[model/effort]`, #4322): titled sections, "Model" and "Thinking Effort", their choices as switches with the current one checked (`MenuChoice` in `apple/Persona/Views/Chat/SessionModes.swift`; a running agent's `SessionModesMenu`, a new conversation's `StarterModesMenu`). Never an inline `Picker` in a titled section: an iOS menu draws no title over its options (`docs/lessons/ios.md`). The Agents header's mode menu (#3719) uses the same switches.
- Dictation bar (iOS, `apple/Persona/DesignSystem/DictationBar.swift`): the tapped mic turns the capsule into `[✕] wave` and ■ ↑ laid over the one-line composer. ■ (stop: the words go into the draft, nothing is sent) takes the place where the mic was tapped, so a second tap there stops (#9619): `[■] [↑]` where something followed the mic (voice or stop on the line, send in the card), `[↑] [■]` where the mic ended its row. This overrides the bar's first layout (#5843, #7586), which put ↑ in the mic's place: a second tap on the mic sent words the owner had not finished, and the owner asked for the mic to switch off where it switched on. ✕ and ■ are muted round buttons (`ComposerRoundButton(muted:)`, `Theme.Shell.roundMuted` or BeautifulUI `inset`), ↑ the ink one; a spinner replaces the symbol while the last words are transcribed (`busy`). Wave and motion tokens: `Theme.DictationBar`, `Theme.Motion.dictation…`. While listening only the wave changes: a `Canvas` redrawn per sample (`TimelineView(.periodic)`) and slid by Core Animation between samples; Reduce Motion stops the slide.
- Accessibility: every control labelled. A mic says "Listening…" while it listens and its hint says what a tap does now (start, or "Tap to stop."), and that it can be held; its activation is the tap (`micAccessibility` in `apple/Persona/DesignSystem/MicPress.swift`). Reduce Motion read in the bar.
- Source: `apple/Persona/DesignSystem/ShellComposerBar.swift:11`, `apple/Persona/DesignSystem/ComposerButtons.swift`.
- Platforms: web composer (14 px radius, 28 px buttons, `web/src/views/chat/Composer.tsx:648`); Android `ComposerFrame` (radius 32 in Glass) with its round button in `ComposerActionSlot`.
- Web and Android text growth (owner-requested correction, #5645 / #5648): grow with the draft to six visible lines,
  then scroll inside the same field. Keep the caret visible when typing or moving it; unrelated updates never take
  over a manual draft scroll. Web reserves room for its toolbar in exceptionally short visible viewports (35% input
  share); that safety cap is not the old 22% cap that stopped an ordinary keyboard-sized phone at three or four lines.
  The existing phone-only full-screen editors are unchanged; their control colors and text placement were inspected,
  not redesigned in this correction.
- Pictures and videos (iOS `mediaPicker`, `VideoAttachmentsBar`, `PasteImageChip`): + › Photos takes pictures and videos,
  four together. A video waits above the field as a row with the video mark, its name and, once on the computer, "size ·
  length", and ✕. While the clipboard holds a picture not yet offered, the focused composer offers **Paste** (the accent's
  capsule on iOS, a Secondary button on Android) and ✕ **Don't paste** at its top, once per copy; the field itself pastes
  only text. Android: the video row in every style, the Paste chip on Glass (`ComposerPaste.kt`).
- Files (iOS `FileAttachmentsBar`, #8361): + › Files never types into the draft. Each file is a card in a row above the
  field that scrolls sideways like the photos' strip, 6 pt apart: `box56` tall and `box160` wide (a new size: a name of
  about 14 characters whole, and the next card showing) on `Fill.well` with the card radius,
  a `well`-sized tile of the file's kind (`FileViewer.symbol`, accent tint; `danger` and the error mark once failed), its
  name on one line truncated in the middle, and under it how far it got ("1.3 MB / 2.4 MB"), Waiting, its size, or
  Failed · Tap to Retry, a thin accent bar along the bottom while it waits or uploads, and the photos' ✕ at the top
  corner (hit through `tapTargetOutset(.all)`). Up to 10, three uploading at once; a tap on a failed card tries again. Send waits for them as for a video.
- Web bottom row (#5324, cindy's toolbar at the owner's request; cindy wins over this section's earlier rules for it):
  `[+][permission picker] … [model pill][Stop][voice][send / queue / stop]`. The + and the picker are bare 30 px round
  controls that show the model pill's frame (`line-strong` border, `hover` fill) on hover or while open; the action is one
  dark 28 px circle (`ink` fill, `surface` icon, 40% when it can't be pressed): a filled plane for Send (**Queue message**
  while the Agent works or needs you), a 10 px square for Stop (never red), round voice in an idle empty box, the main
  conversation's not-ready button as a pill in the same fill. With something typed while the Agent works, a second Stop
  sits left of voice. No @ or / button: typing them opens their menus. The model pill is the `trailing` slot and the
  picker the `leading` one, in an Agent chat and in a Persona chat (#5033; the new-chat page has no picker). The card is the
  `composer` container whose width sets the pill's and the picker's modes ("Model pill and unified model panel" in 3.8).
- Web + / @ panel (#5324, `ComposerPanel.tsx`): one panel for both entries, 480 px and never wider than the composer
  (full width on phones), 12 px corners, `line` border, raised shadow, at most 400 px tall. From the + it covers the box
  from its bottom left with a pill search box on top. Typing @ anchors it above the box when space permits. A 24 px
  header (12 px medium, `ink-3`) names each populated group. Rows are 36 px tall (44 px on touch), with a 16 px icon,
  a 14 px name and 12 px secondary text. The slash menu's glide fill marks the active row. A quiet status line reports pending data while local options stay usable. An empty search shows "No matching items".
- Web permission picker (#5324, `PermissionPicker.tsx`): the mode's symbol (14 px), its name (13 px, truncates) and a
  chevron; Full access tints the words `red`, Auto-review `mode-acts`; under a 300 px composer (a 320 px phone) only
  the symbol. A mode not reported yet is named by no value: the raised hand and "Permission mode", no tooltip (#5756). Its menu is the shared `Menu` (300 px) with each mode's symbol, name and check; descriptions are tooltips
  (`MenuItem.tip`), except a disabled row's reason and a row asking again, which stay under the name.
- Permission mode names on every client (#10363) come from `spec/fixtures/permission-modes/names.json`: Default
  permissions, Auto accept edits, Plan mode, Auto-review and Full access in en, zh-Hans and ja. Pickers, settings,
  planned tasks, answered-plan labels and confirmations use those names; approval-action phrases, launcher-default
  choices and unset/default-behavior labels stay distinct. The fixture README names each client's drift check.
- Web under the box (#5324): the source and folder line at the left (12 px `ink-3`, a folder mark, truncating, full
  values on hover; an ⓘ menu at phone width), the context meter at the right.
- Web `/` palette (#5325, `web/src/views/chat/SlashPalette.tsx`; cindy's slash command palette, followed at the owner's
  request, 2026-11-12): a 320 px panel anchored at the composer's left edge above it (`rounded-xl`, `bg-surface`,
  `shadow-overlay`, 6 px padding, at most 400 px or half the visible height, scrolling), 36 px rows (44 on touch) with
  the command in 14 px `ink` (medium on the focused row, under the gliding `bg-hover` highlight) and its source in 12 px
  `ink-3` on the right (none for Persona's own, the agent's name, an extension's name). The focused command's description
  is a 280 px card (`rounded-xl`, `shadow-overlay`, 14 px padding; title 14 px medium, text 13 px / 1.5 `ink-2`, what it
  types in `ink-3`) 8 px beside the panel, right or left, or a line under the rows when neither side has room. Empty:
  **No matching commands**, 40 px, centered, `ink-3`. No key hint line. Ranking, keys and deviations: `docs/web/web.md`
  ("Input (Prompt Bar)").
- Web skill selection (#11247): the Assistant and Agent composers use the same up-to-480 px menu for `@` and `/`.
  Group headings, scrolling, the glide highlight, skill rows and the detail card come from the same picker. All enabled
  skills in the scoped catalog can be browsed or searched. The slash menu keeps up to 25 ranked commands first.
  A skill row shows its name and short description. The focused skill's detail shows the full description and the source
  supplied by the node, including any visible directory. It fits beside the list where space permits and scrolls beneath
  the list on a phone. The list fits the space around the composer and the keyboard. Selecting a skill adds an opaque
  context chip. Opening either picker refreshes the inventory and can display the composer's last same-scope result
  while waiting. Fresh results preserve the selected identity while it is available.
- Editing state (web, #4503): editing the last message puts its words and quotes back into the box under a one-line bar at the box's top, above the context chips and quote bars. The bar has an accent pencil, **Editing last message** (`ink-2`, medium), a truncating hint (`ink-3`, precise pointers only) and a × with the quote bar's 24 px look. It is the only sign of the state, and the text field stays the same field. The irreversible step (the rewind) waits for an explicit send, which the bar's hint names. Esc or × leaves with nothing changed (`docs/web/web.md` "Pages", **Edit last message**).
- Queued messages (web, #5327): drawn as cindy's pending queue at the owner's request (where cindy and the older design
  differ, cindy wins for this area). They sit in the composer's `top` slot, first in the card, flush with its border and
  edges, with a `line` hairline between them and the input; the card's own border and focus ring hold both. List: at
  most 196 px, then it scrolls; padding 4 px sides, 6 px top. Row: 8 px radius, 2 px vertical padding, 6 px gap, one
  line of 13 px / 1.25 text, `ink-2` at rest and `ink` with the `hover` fill while hovered or focused; no visible
  number (labels carry the index). Left, a 20 px drag handle (`grip` icon, 12 px, `ink-3`) shown with the row's actions;
  right, 20 px round buttons with a `line` ring on `surface` (↑ Send now 12 px, pencil and trash 11 px), shown on hover
  or focus and always on a screen without hover, a spinner in their place while the row's request is out. Send now acts
  at one press (4.4), its tooltip saying what happens. Messages from
  other conversations come last, folded into one 32 px line: a `chat` mark in the rows' 20 px handle column, **N messages
  from other chats** (12 px, `ink-2`, medium) and "· Arrives when this chat is free" (`ink-3`), a chevron at the end.
  Opened, each row names the chat it came from once (medium, `ink`, at most 40%; the hub's 「来自「…」：」 prefix taken off)
  and then its words, with no mark, handle or buttons; a line cut short has a chevron and opens in place to its whole
  text (at most 132 px, then it scrolls, selectable). Only the groups that are there are drawn: with none of the owner's
  rows the relayed line is the card's first thing, with no hairline over it (#9749). Four rows or fewer show in full; from five, three and a centered 12 px **N more** / **Show less**
  with a chevron under a `line` divider. On a phone on its side the queue is one "N queued" line beside the run's status
  that opens the same rows. Spec: `docs/web/web.md` ("Queued messages").
- Duplicates: four composer implementations exist (DS-19).

#### Large-window hardware input (Android)

`LargeWindowComposer` adds hardware actions without replacing `ChatComposer` or changing its visible layout. In regular / expanded windows, Ctrl or Meta+Return is the same action as Send and uses the same enabled, content, attachment-readiness, sending and blocked gates; the first key-down acts once, repeats and key-up do not. Return and Shift+Return insert a newline, and an active IME composition passes every key to the IME. `LocalComposerHardware` supplies the optional actions to `ExpandedComposer`: send uses a separate collapse path that does not request small-field focus and then invokes the caller's one `onSend`, while Escape collapses and restores the small field's focus. Null is the default and preserves phone, preview and standalone-editor behavior. `/permissions` is the one send that keeps the small composer expanded: its leading mode control stays composed for the picker's whole lifetime, not only until the open request is consumed. Compact and wide-phone windows do not install these actions; the neutral outer composition identity remains stable across window-flag changes so uploads and focus are not recreated. Ctrl+/, terminal keys and unrelated keys are not consumed by the composer.

#### Large-window file drop (Android)

`ComposerDropTargetFrame` is the composer's drop-target chrome: a style-shaped accent outline (`Tokens.Size.lineMedium`, existing motion standard / reduced-motion snap) with no padding or size change and no enabled compact / wide-phone target. Its semantics say **Drop files to attach them** while hovered and **Copying dropped files…** during import; all copy is localized. A target accepts only a visible, writable chat with an upload service. `ComposerDropImport` copies a transient-grant URI into app-owned `cache/chat-drops` before calling the ordinary attachment selector and uploader. Media count, 200 MiB, JPEG conversion, video / file bytes, progress, failure, retry and send remain the ordinary Composer rules. `ComposerDropCleanup` follows the draft rather than composable presentation, removing only unused app-owned copies even after window-flag changes, never a directory or another draft's source.

**Run status bar** (web, #5312; the owner asked on 2026-11-12 that it follow cindy's `RunningStatusBar` and
`RunningTokenRatePopover`, and cindy wins over what this document said before; `web/src/views/chat/RunningStatusBar.tsx`,
`docs/web/web.md` "Live status and streaming"). One row above the composer while a turn runs, 13 px medium, 8 px side and
6 px vertical padding, its two sides 12 px apart:
- Left: a 14 px mark and a short state in `--status-accent`, truncating. The sparkle (layers while compacting, activity for
  background subtasks, a 2.5-stroke check for **Done**). An Agent session says what the turn does in cindy's words
  ("Thinking…", "Generating…", "Editing files…", "Searching the web…", "Spawning a subagent…", "Bash running…",
  "Compacting context…", "Working…"); a Persona conversation says it in cindy's plain phases ("Thinking…", "Replying…",
  "Reading files…", "Updating files…", "Searching files…", "Searching the web…", "Reading web pages…", "Organizing the
  conversation…", "Working…") and never names a tool (super rules 0 and 4, section 5.6).
- Model retry: while the screen reports one, the left leads with the existing classifier's plain reason and localized
  `Retrying N/M`; the observed error is secondary detail beneath it. It is never retained from the last status: ordinary
  work, `live: null` and turn end remove it before the usual linger/fade (#887).
- Right, in `--ink-3`, tabular figures: the time in the work summary's format (`0s`, `1m 0s`, `1m 5s`, `1h 0m`; section 4.7),
  then in an Agent session either `· 42 tok/s` or `· ↓ 1.2k tok` (a 13 px arrow mark), never both, and no speed or tokens
  before there is a measurement (principle 5). The time and speed together are a 24 px round button with `hover:bg-hover`
  (a 44 px touch target, `touch-hit`).
- Speed card: 320 px wide (the screen less 16 px each side), 8 px above the button and right-aligned with it, a `rounded-xl`
  `bg-surface` card with the hairline border and `shadow-overlay`, 16 × 12 px padding. "Latest speed" (12 px `--ink-2`, the
  activity mark), the number 28 px medium with "tok/s" 12 px `--ink-2`; beside it a 128 × 48 sparkline in `--ink` (a 12 %
  baseline, an 8 % area, a 1.5 px line and a 2.5 px dot at the latest sample); below, three 12 px columns, label in
  `--ink-2` over a medium value: Turn average, Output, Peak ("—" until measured). Hover shows it after 500 ms as a
  tooltip; a click pins it with a 24 px round ✕ in its top right corner (44 px to a finger); ✕, Esc and a second click
  close it, a click outside does not. While pointed at, focused or pinned the button stays when the speed goes. It opens
  with the cards' `pop-in` and goes with their fade.
- Done: when the turn finished (not waiting on you, stopped or cut off by an error, which keep their last words), the
  linger second shows the check and **Done** with a 0.4 s pop (scale 0.85 → 1.08 → 1, transform only, none with
  reduced motion); then both sides fade out over 0.4 s, except a pinned card's button. The row breathes once (opacity to
  0.45 and back over 1.5 s) when its words or count move.
- Kept from Persona where cindy differs: the background workflow line ("Background workflow · Verify · 169/185+", timed
  from the run's start, the workflow mark in `--ai`), the still waiting line (#6045) and **Disconnected ·** in front of the
  last state. Left out: cindy's **Stop all** for background subtasks (#5044) and its host-measured speed (Persona measures
  from the status line's count instead, `web/src/lib/token-rate.ts`). New mark: `activity` (Lucide's activity, as cindy's).

### 3.2 Message bubbles

- User bubble: trailing, radius 20, padding 16 x 12, `ChatBox.user` fill in the unified layout, otherwise the theme's bubble gradient (`apple/Persona/DesignSystem/QuoteViews.swift:295-345`, `apple/Persona/Views/Chat/MessageViews.swift:151`).
- Assistant text: unboxed in Agents chats; boxed in `ChatBox.persona` in Assistant chats. Streaming text re-parses only the tail.
- Quotes sit above the words and jump to the original on tap.
- Your own long message folds (#5676; iOS `BubbleFold`, web `BubbleText`): past 14 lines as drawn it shows 10, the last
  line fading, with Show More / Show Less (展开 / 收起, 展開 / 折りたたむ). iOS puts it inside the bubble at its bottom, in
  the bubble's link color (`FoldToggle`), folds past 9 lines to 5 at the accessibility text sizes, opens a folded bubble
  on a long press too (section 4.2), keeps open or not per message, and holds the bubble's top in place while it opens
  or folds. What opens fades in under the lines already shown with `reveal`, as a tool's details do (`.unfolding`,
  #6481); the fold's fade mask grows open with it. The lifted copy is always its whole height, also after Show More
  (#6481). The web puts the button under the bubble and its fold scrolls inside; it has no long press to open and
  does not keep open or not (noted in #5676 for a web issue).
- Everything in the user's bubble takes its colors from the bubble as drawn (`Theme.UserBubbleInk`, #5204): the
  unified layout's box (primary words) or the theme's bubble, never the style alone. Blocks at its top (quotes, forwards,
  text to change, comment blocks) draw their rule, heading and words in the bubble's words color, on the accent's tint
  in a pale bubble or the box, and on a 12% black shade in a colored bubble: a veil of white words would lighten what
  they sit on. Floors (`StyleSpecTests.whatSitsInTheUsersBubble`): the block's words 4.5:1 on its fill over the
  bubble's top and deepened bottom, linked paths 3:1.
- Forwards (#3807) use the quote card with the forward arrow before the heading ("Forwarded from Claude · chat"), a line
  for the computer and time, and four lines of the original; a tap opens the original chat at that message. Over the
  composer the same row adds a line saying where it comes from and goes. Forward takes Share's place in the message bar
  (the bar keeps its width); Share as Image is the first row of the Forward To sheet. Web (#3899): Forward takes Hand to
  Persona's place in the message actions; the card over the composer (`ForwardBar`) and in the bubble (`ForwardBlock`)
  are the quote bar's and quote card's shapes (`views/chat/Quote.tsx`) with the `forward` icon, and Forward to is a
  `Dialog` (a bottom sheet on phones) with palette-style rows; no Share as Image row, as the web has no share card.
- Forwarding several at once (#3838): the entry is a row at the top of Forward To, "Select Messages…" (「多选…」), never
  a bar item next to Select Text, and the message bar keeps its width. Selecting in a chat is the Inbox Select's pattern:
  a round mark on each message's leading edge (filled accent when picked), a tap anywhere on the message toggles it, the
  composer gives way to a bar with Cancel, "N Selected" and Forward N (disabled at 0), and Cancel leaves the chat as it
  was. Several forwards of one chat are one row over the composer (a line each, one ×) and one card in the bubble (each
  message's author, time and two lines, each opening it; past three, "N More"). Web (#3838): "Select messages…" is the
  first row of Forward to; while selecting, each message with a Forward has a checkbox (`ui/CheckMark`) in its first
  place, a click anywhere on it toggles it and nothing inside it answers (no hover bar or menus), and the bar in the
  composer's place reads Cancel, "N selected" and Forward N; Esc cancels too. The row over the composer is
  `ForwardGroupBar` and the bubble's card `ForwardGroupBlock` (`views/chat/ForwardGroup.tsx`), past three "N more".
  Android (#5184) opens selection from Forward to. Its Inbox checkmarks take the whole message as a target, and the
  target composer and conversation use the same three-message group preview as the web and iOS.
  Web (#9469): the message menu ends in "Select messages" (「多选」) under a divider with the `check` icon, as iOS; the
  bar reads Cancel, "N selected", the Include questions chip (a round `CheckMark`, the accent tint when on) and Forward,
  Copy and Export image (Export image the primary button); on a phone the three are a card under that line, an icon over
  each word, as the iPhone's long-press bar. A question a picked reply brings has a lighter tick (`bg-accent-tint`).
  Export image is a `Dialog` with its ✕ (a bottom sheet on a phone): the preview, then the switches, then Copy image and
  Download PNG in the footer; the picture's foot is the assistant's face and name with "(Persona)", as iOS's
  `PersonaBrandLockup` `.mark`.
- Share as Image (iOS `MessageShareSheet`): a full-screen page with the message as a card (who said it, where and when, the
  words with code blocks that wrap and have no Copy bar, a quiet Persona mark, on a pale blue-to-lilac gradient), Light /
  Dark at the top, and Share (the picture), Save, Copy and Share Text along the bottom; the card is 390 pt wide, wider for
  tables, code and very tall cards, at most 520 pt (`ShareCardWidth`). Android has no Forward: on Glass the message
  menu's Share opens the page (`MessageSharePage`, Light / Dark in the top bar, the picture drawn from the card's
  `GraphicsLayer`); Brief and Board share the words. What you see is what you share: a reply's cards are drawn as the
  chat draws them, with nothing to use, and a card that only draws live is its fallback, never its JSON (#7261,
  `docs/assistant/assistant-views.md` "Sharing a reply as a picture").
- A message sent from here and not in the transcript yet (iOS `PendingBubble`): drawn as a sent bubble at once, with a
  mark before it, level with its last line: a quiet clock while it sends; a clock while it waits for the network, whose
  small menu is Retry Now and Don't Send, with "Sends when connected" under the bubble; a red mark once it failed, with
  Resend and Delete. Waiting or failed, the bubble is at 0.6. The long press ends in the mark's own word (Don't Send or
  Delete). A failed one with messages sent after it sits before the first of them, at the end of the exchange it was
  written in (`ChatLines.placeFailed`; Android in every style, `placeFailed`). Android draws the bubble so on Glass
  (`OutboxBubble`, #4339 C9) and keeps its status line for a delivery the phone has to check.
- Files sent with a message (iOS `AttachedFileCards`, #8361; web `FileCards`, Android `FileCards.kt`): above your bubble,
  right-aligned, one card each, at most `box280` wide: a video or a recording plays (`AttachedVideoCard`), any other file
  opens the file viewer (`AttachedDocumentCard`: an `avatarL` tile of its kind in the accent's tint, the name in
  `secondaryMedium` truncated in the middle, its size in `label`; Copy Path on a long press). More than four show three and
  an "N More Files" row (`tapTarget` tall) in the accent color that opens the rest in place (Show Less folds them). A file the computer no
  longer has is dimmed to 0.6 with the missing-file mark and "No longer on the computer", and takes no tap.
- The images a turn's work looked at (iOS `ViewedImagesRow`, docs/protocol/protocol.md "Images viewed this turn"): bare
  thumbnails between the work line and the answer, `box64` tall in a strip that scrolls sideways, one alone `box80`, each
  as wide as its picture (¾ to 1.6 of its height), with a small mark for a screenshot or a web page and nothing written
  around them; a tap opens the viewer on the turn's images. Android draws it on Glass (`ViewedImagesRow`, #4339 C9).
- Pictures in replies (#9474; web #9475, Android #9476; iOS `ReplyPictureRun`, rules in PersonaKit `ReplyImages` and
  `ReplyImageLayout`, tokens `Theme.Size.replyPicture` / `replyPictures` / `replyPictureFailed`): a web picture on a
  line of its own, however it sits in its paragraph, is a block picture; block pictures with only line breaks or blank
  lines between them are one run. One picture is `replyPicture` (240 pt) tall and as wide as its shape from 3:4 up to
  the column (wider or narrower ones cut around their centre), never larger than itself; two are squares side by side;
  three or more are squares three to a row (four as 2 × 2), at most two rows, the sixth dimmed with "+N" for those not
  drawn; a run is at most `replyPictures` (480 pt) wide. Same left edge as the text, `Radius.control`, the hairline
  `cardStroke`, `Fill.well` while it loads. The box is its final size before the picture arrives: only a lone
  picture's width changes, so the text below never moves. One that did not load is a `replyPictureFailed` tile with
  the retry mark (in a run, that square), a tap retries; a long press offers Open Original. A picture inside a line of
  text is the line's height. A tap opens the viewer on every picture of the reply. They load through the hub's remote
  image route (`remote_images`, docs/protocol/protocol.md "Remote images (#9473)") at the size drawn; what other
  people wrote (the Inbox's Lark, GitHub and connector threads, `asksBeforePictures()`) keeps each run its size with
  Load Image over it until tapped, since a picture there may be a tracking pixel.
  A machine-local Markdown image uses the session's machine and cwd and the same layout. A local image inside a line
  of text is lifted into its own block so the machine-file loader and any failure are visible. A failed local image
  keeps Retry on its tile and shows the complete reason below the tile or run, including at accessibility text sizes;
  recreating the tile does not retry, and Retry runs only after a tap. The full-screen viewer shows the same reason
  and offers Retry. Local file links open the same file viewer as bare paths, including missing or denied files.
  The web (#9475) draws the same rules: `ui/ReplyImages.tsx` (a chunk of its own with the step that lifts a run out of
  its paragraph, `ui/reply-images-plugin.ts`, loaded with a reply's first picture), rules in `lib/reply-image-layout.ts`;
  a lone picture is 300 px tall (240 px in a narrow window), a run at most 480 px wide, the turn images' 10 px radius
  and `shadow-hairline`, `bg-field` while it loads, sizes as inline styles (no new utility classes on the first
  screen). Markdown rendered with `askImages` (Inbox connector and Lark threads) keeps a run its size with Load image
  over it; a hub without `remote_images` keeps every picture behind a click (#8162's gate), loaded from its host with
  no Referer.
  Android (#9476) uses a 240 dp single and a 480 dp maximum column in Glass, Brief and Board. Inline pictures use the
  measured text line height, including at large system text sizes. A tap opens that reply’s pictures in the existing
  viewer, including inline and overflow pictures. Each third-party thumbnail and previously unopened gallery page
  asks before loading. Mail keeps its Show images action. A pairing that has yet to advertise `remote_images` waits
  for a tap and uses a separate HTTPS reader. WebView link preloading skips picture-bearing replies.
- No shared bubble component exists. Messages, Tasks and Lark rebuild it with different paddings and gutters (DS-20).
- Web: user bubble 14 px radius; replies unboxed. Android: 20 dp in Glass, max width 86%.
- Web reply bar (#5314, `ReplyBar`; the owner's requested alignment with cindy's `MessageActionBar`, 2026-11-12): only
  under a finished turn's last reply, never under narration in a work summary or words still streaming. Copy message ·
  Start a new session ("Start a new chat" in Persona conversations) · ⋯ More message actions (Quote, Forward, Use as
  context) · the relative time (12 px `--ink-3`, the exact time as its tooltip). 24 × 24 round buttons 2 px apart, 14 px
  icons, `--ink-3`, `--ink` on `--hover-2` under the pointer. Shown on hover or keyboard focus, it stays 250 ms after
  the pointer leaves and fades over 250 ms; an open ⋯ keeps it. Touch screens keep the copy + ⋯ row under that reply;
  the right-click / long-press menu keeps every action. A reply cut short by an error or an interruption carries
  cindy's "This reply did not complete" line above it (13 px `--ink-2`). After the time (#5426, `TurnUsage`, cindy's
  usage cell): what the exchange cost, 6 px on, 12 px `--ink-3`, half a pixel down, "Worth $0.12" in an Agent session
  with cindy's usage card (320 px, `--surface`, `--line` border, 12 px radius, `shadow-overlay`, 16 px padding, 12 px
  `--ink-2` lines; under the text from its start, above it when the chat has no room below; 250 ms into a hover, held
  on the card, gone 220 ms after the pointer leaves, pinned by a click, Esc or a press elsewhere closes it, the bar
  stays while it is open), or without a cost "23.4k tokens" with their lines in the dark tooltip (`--tooltip-bg`,
  13 px, at most 420 px, 500 ms into a hover, never pinned by a pointer). A Persona conversation shows only "≈ $0.12"
  and one plain line, and nothing without a cost (super rules 0 and 4). On touch screens the text ends the copy + ⋯
  row: a tap holds the card or the tooltip, and a second tap on the text or a press elsewhere closes it.
  Translate (#8650) loads a hub-paged reply's full text before requesting its translation: the original and its
  full-text loading/retry control stay visible until the complete translation succeeds. Show original restores
  the cached complete reply, not an unmarked prefix; failure to load the full text never sends a partial translation.
  Details: `docs/web/web.md` ("Reply bar").
- Web hover bar beside your own bubble (pointer only: the time, Copy message, Quote, Edit, ⋯): left of the bubble, 6 px
  away, where it fits inside what shows the chat. Where a narrow window leaves a wide bubble no room there, the same
  buttons sit under the bubble, right-aligned, as a small floating bar (`rounded-control`, `bg-surface`,
  `shadow-hairline`) over what comes next; a transparent bridge over the 4 px gap keeps the hover, and hidden it takes
  no clicks. Where it goes is measured when the pointer or the focus comes into the bubble's row, and only its classes
  change, so the button Tab reached keeps the focus (`docs/web/web.md`, "The hover bar beside your bubbles").
- Android Markdown tables in chat and file previews (`ReadableMarkdownTable`): a table that fits the upstream renderer stays unchanged. In a wide one, short columns keep their natural width, long columns wrap between the font-scaled `RichTextTokens` minimum and maximum, and only the table scrolls sideways. Share pictures keep the unchanged static table.
- Android tool diffs (`DiffView`, an Edit / MultiEdit / Write call's detail): as iOS `DiffView`, an added or removed line sits on the palette's diff fill (`diffAddFill` / `diffDelFill`: Glass reads `Tokens.Common.<Light|Dark>.Diff`, iOS `Theme.Diff.addFill` / `delFill`; Brief and Board keep their own diff hue at the tokens' strength), the words that changed in a paired line on the deeper emphasis (`diffAddEmphasis` / `diffDelEmphasis`, the generated `Tokens.Common.<Light|Dark>.Diff.addEmphasis` / `delEmphasis` from `spec/tokens`, iOS `addEmphasis` / `delEmphasis`; the older `diffAddWord` / `diffDelWord` stay for the extension review), the sign and the +N −N keep the plain diff colors, and the code stays the label color. Long runs of unchanged lines fold into one quiet row until Show All; a Codex patch shows whole, on plain rows with no tint and no word marks. No second diff component: the Git page and the extension review keep their own surfaces.
- Web Markdown tables in replies: a column is as narrow as its words. Headers never wrap and have no minimum width; a
  cell whose words fit one line in about 5.5em stays on one line with no minimum; only longer cells keep 7em, so a
  squeezed table never stacks a sentence a word per line. The copy button (hover or keyboard focus; always on a screen
  without hover) sits on a cap as tall as the header row in its `--inset` color, with a short fade on its left, and the
  last header cell keeps room for it, so it never covers header words (`docs/web/web.md`, "User bubbles").
- Model change divider (web, #5034, `ModelChangeDivider`): where an exchange's reply came from another model than the
  previous visible reply, between that exchange's message and its reply: two `--line-strong` hairlines and a centered
  pill chip (`⇄` 12 px, "Opus 5.5 → Sonnet 4.5", 11 px `--ink-3`, `--line-strong` border, `bg-page/50`, padding 10 × 4),
  `role="separator"` with "Model changed from … to …"; not a control. Only the transcript's record of the replying model
  counts, never the picker (`docs/web/web.md`, "Model change divider"). Follows cindy's model-only switch chip at the
  user's request (2026-11-12), with model names instead of raw ids.
- Engine switch divider (web, #5054, `EngineSwitchDivider`): above the first message a conversation's other engine got
  (Claude Code ↔ Codex), the model change divider's row with a chip "⇄ Switched from Claude Code to Codex · <model> ·
  <badge> ›": the badge "session resumed" when the engine's earlier session was resumed, else "Continued from summary".
  When the switch carried a handoff the chip is a button (tooltip "View handoff context", `aria-expanded`) that opens a
  panel under the line: `rounded-card`, 1 px `--line-strong` border, `bg-inset`, padding 16 × 12, an 11 px medium `--ink-3`
  title, the handoff in 12 / 1.55 monospace `--tool-ink`, at most 16 rem high, scrolling, selectable; opened or not is
  remembered per divider. `role="separator"` is the left hairline, labelled "Switched from … to …". Under 640 px the chip
  reads "Claude Code → Codex · <badge>". That exchange draws no model change divider. Follows cindy's agent switch card
  at the user's request (2026-11-12), with the model's name instead of its raw id and the badge always shown
  (`docs/web/web.md`, "Engine switch divider").
- Mode change line (web `ModeChangeDivider`, iOS `ModeChangeLine`, Android `ModeChangeDivider`, #10297): a permission mode change the hub recorded
  (`docs/protocol/protocol.md` "Session mode changes"), at the end of the exchange it happened in, in an Agent's chat
  only. The compaction divider's look: hairlines either side (web `--line-strong`, iOS `Theme.cardStroke` / separator,
  Android `PersonaTheme.palette.line` / `Tokens.Size.line`) of one line of words (web `text-submeta` `--ink-3`, iOS
  `Theme.Font.label` secondary, Android `PersonaTheme.type.micro` `ink3`), not a control. Words: who (the
  device's name; "The hub"; "A device"), the modes in quotes (zh-Hans and ja 「」, en “ ”), a pick that waits for a
  restart says so, and what became of the restart says why in plain words from the node's reason code; no reason, no
  colon. Hairlines only where the words fit on one line with at least 16 px / `Theme.Space.l` / `Tokens.Space.l` of
  rule each side; otherwise the words alone, wrapped and centered (the owner, 2026-11-18: rules beside the first of
  two lines read as crooked, rules a pixel or two long as dots). The web loads its code and words (`chat-modes`) with
  an Agent's chat, off the first screen. A frozen window shows only changes up to its last shown item; newer changes wait
  with the newer transcript. The record is read when the chat opens and when its mode or waiting restart
  changes, never on a timer, a refresh or the app back in front.

### 3.3 Cards and surfaces

| Component | Use | Source |
| --- | --- | --- |
| `.surface(in:)` | floating panels: composer, strips, toasts, notices | `apple/Persona/DesignSystem/Styling.swift:8` |
| `.card()` | grouped-row card, padding default 14 | `apple/Persona/DesignSystem/Components.swift:429` |
| `NoticeBanner` | persistent in-page notice, neutral, caution or danger, one action | `apple/Persona/DesignSystem/Notice.swift:7` |
| `ScreenMobileDataCard` | the live screen's one question on mobile data (#6396): a `.surface` card under the bars, covering none of the picture, no wider than `Size.readable`: the network in its title (mobile data or a personal hotspot), one line, two choices with about how much data an hour each takes (prominent Clearer, plain Data Saver; side by side where they fit, stacked at large text, never wrapped) and ✕. A `NoticeBanner` has one action; this is a choice between two. The web's (#6558, `MobileDataCard`) has the same parts under the page's bar, no wider than 420 px, in the stats panel's surface; it stacks the choices where they don't fit side by side (measured, as `ViewThatFits`), and ⋯ holds the choice, since the web has no Settings row for it | `apple/Persona/Screen/ScreenMobileDataCard.swift`, `web/src/screen/VideoPicture.tsx` |
| `DispatchHandoffCard` | one card per dispatched task in the chat: ↳ well, title, New / Already Active / Resumed badge, last-active time and state, ↗; a failed start says why in red; one tappable element (`dispatch.task.<id>`) | `apple/Persona/Views/Assistant/Tasks/DispatchHandoffCard.swift` |
| `PlanBriefView` | a plan task's brief before Start: who asked (two lines), one summary line, Show All for the goal, criteria, time and limits | `apple/Persona/Views/Assistant/Tasks/PlanBriefView.swift` |
| `CardForm`, `CardFold`, `CardCode`, `CardPlan` | a pending card's form: what it asks in one scroll area over its answers, which stay in view; a long command, terminal text or plan folded to its first lines (fewer when the card is short), the last one fading, Show All N Lines / Show Full Text opening it whole (section 4.8, #6605) | `apple/Persona/Views/Interactions/CardFold.swift` |
| `ReadinessCard` | what the main conversation still needs | `apple/Persona/DesignSystem/ReadinessCard.swift:8` |
| `PlaceMapCard` | where the phone is, or the places a location tool found, as a map under the turn's work line | `apple/Persona/Views/Assistant/Tasks/PlaceMap.swift` |

`.card()` uses the system grouped background, not the style's card color, so it does not follow Clay, Island or Lark (DS-07). The space page's cards (`SlotCard` and the dashed new-tab card in `apple/Persona/Views/Chats/SpaceView.swift`) are `.card(padding: 12)` with a minimum height of `box150`; Android Glass draws the same page with `AgentSlotCard`, `AgentNewTabCard` and `AgentCardRow` (`android/core/designsystem/src/main/kotlin/persona/core/designsystem/AgentSpaceCards.kt`), on its opaque card color with the hairline stroke, since its translucent card color is nearly invisible on the dark page (#4338). The page of a computer's projects that the space page's computer opens (`apple/Persona/Views/Chats/MachineSpacesView.swift`) is an inset grouped list: a summary row, a Working section and an Idle section, each with its count in the state's tone, each project a row that opens its space page; Android sets the same rows in `JGroupItem` cards and `JRow`s (`AgentMachineSummary`, `AgentProjectLine`, `AgentIdleProjectLine`, #4338 A11); in dark the cards are the system's lighter gray on the black page (`JGroupItem(grouped = true)`, the opaque card color, as Settings' grouped surfaces), since the translucent glass fill is near-invisible there. Inset wells (`Fill.well`) are rebuilt by hand in 24+ places. Web has no shared card outside the admin app.

**Place map card** (iOS, #5543, `PlaceMapCard`): a quick result like the added-reminder card (`QuickResultCard`): the same `Fill.well` and `Radius.card`, under the turn's work line, with the tool calls still folded (super rule 2). On top, a still map `Size.chatMap` tall (160, scaled), made by `MKMapSnapshotter` off the main thread for the card's size and appearance and cached (bound 24); while it is made the well's fill holds the same height, so nothing below moves. Places carry `CountBadge` numbers on the accent in the order the tool returned them, which is the order the answer counts in; the phone's own fix is an accent dot. Under the map, the first three places as rows (number, name in `secondaryStrong`, distance and address in `caption`) with an icon-only Directions button (`Icon.directions`, 44 pt target, spoken "Directions to <place>"), then "N more on the map". Tapping the map opens a sheet with a live `Map`, the same numbers as `Marker` monograms, every place's row and Done. Why a new card: no existing component shows places, and a live map in the chat would redraw while scrolling (`docs/apple/performance.md`, P8). No setting; nothing is drawn for a failed, empty or running call. **Hand-off** (#5545): directions `phone_maps_navigate` opened in Apple Maps are the same card with one place, the destination: on the map a white `Icon.place` in an accent circle (the accent dot stays the phone's own fix), and one row with Maps' app icon (`BrandMark`, `Brand.iosMaps`, as Settings › Connectors draws it), the destination's name, the mode and "Opened in Maps" in `caption`, and the icon-only button (`Icon.directions`, spoken "Open directions to <place> in Maps") that opens the same directions in the same mode again. Nothing opens Maps without a tap, and the words say Maps navigates, not Persona. **Route** (#5544): a route `phone_maps_route` worked out is the same card with the destination: on the map the route's line in the accent (`Size.barWide`, round caps and joins) from the phone's accent dot to the destination's mark, the same line as a `MapPolyline` in the sheet with the same accent dot (an `Annotation` titled "Start") where it begins, since the phone may have moved since; one row with `Icon.place`, the destination and "Walking · 6 min · 420 m" in `caption`, and under it a full-width prominent "Start in Maps" button (`Icon.directions`, `labelStrong`, one line that scales down rather than wraps, spoken "Start directions to <place> in Maps") that hands the route to Maps in its mode (#6177). It is labeled, not icon-only like Directions and the hand-off's button, because starting the route is what the card is for. Transit has no line (Apple Maps gives apps none), only the destination with its time and distance.

#### Adaptive card grids

Machine cards use one adaptive row grid on iOS and Android. Measure the actual pane's available content width after page and reading insets, never the physical display or a device-model bucket. With minimum column `cardColumn` 330 and gap `Space.m`, the column count is `floor((contentWidth + gap) / (cardColumn + gap))`, clamped to at least one; insufficient space stays one column instead of squeezing a card. Android inserts one weighted row at a time into the page's existing lazy list, keyed by that row's stable machine ids, rather than nesting a second scrolling grid. Empty slots in a short last row keep their weights. Cards in a row have equal height and their charts align at the bottom. Reordering is a separate concern projection: equal ranks retain source order, so heartbeat and sample changes at the same severity do not move rows or reset reading position.

An extension's dashboard page (web, #9158; UI version 8's `columns` layout, `web/src/views/extensions/ViewBody8.tsx`) is the same kind of grid on the web: the cards are an overview card's surface (`rounded-card`, `bg-surface`, `shadow-card`) with its head inside (the `text-meta` medium title, an optional 14 px icon in its named color before it, the section's link in `text-label` accent with › at its end), `gap-3` between them, and every card in a row as tall as the tallest. The columns follow the page content's width with container queries: one column, two once each card keeps 24rem, three once three do, so 1440 × 900 with the sidebar open has two. 24rem rather than `cardColumn` (21rem): these cards hold lists whose rows carry a value or a badge at their end, and at 21rem a third column cuts their titles short at desktop widths. A card that spans the row takes all columns. Inside a card that asks for it, rows go two a line from 36rem of card (three from 54rem) without hairlines between them, and stack with hairlines below that. Why a grid and not masonry: the reading order stays the narrow screen's order, left to right then down. Android (#9158) draws the same page with the native rule above (`cardColumn` and `Space.m`, at most three columns, a phone one, drawn as the stack): the page measures its pane, a columns view takes the pane's whole width (`JListPage` `fullWidth`), and each row of cards is one item of the page's lazy list, every card the section's group panel (`JGroupItem`) with its head inside (an icon in its named color, the `headline` title, the link a quiet `JButton`) as tall as the row's tallest. A section's rows go side by side where each keeps `cardColumn`.

On iOS (#9158, `apple/Persona/Views/Settings/ExtensionColumns.swift`) the same page follows this section's rule for machine cards: the page's own width inside its margins (`Space.page`), never the device, with minimum column `cardColumn` and gap `Space.m`, at most three columns. Unlike the Machines grid the minimum grows with the text (`Theme.sizeScale`, as `scaledFrame` does; about 446 pt at the interface's largest size, xxxLarge), so a window that just holds two columns at the default size holds one at larger sizes instead of squeezing cards of large text; a 13-inch iPad's detail column on its side keeps two. One column is the page's list exactly as a stack draws it, so an iPhone upright looks as before. Two or more: a lazy stack of lines, each line a layout that offers every card the line's tallest height (`ExtensionColumnsLayout`), the cards `.card()` with the head inside (icon in its named color, the title in `Font.heading` as the Machines cards', the link in the accent at the end, at least `tapTarget` square). `cardColumn` rather than the web's 24rem: iOS rows are drawn for a phone's width, and 330 pt is about an iPhone's card, so a column never holds less than a phone shows. A section's rows go side by side by the same rule (`cardColumn` each, in the width the rows are given) and stay one to a line below it.

An extension's gauge tile (web, #9337; UI version 9's `Tile.gauge`, `web/src/views/extensions/ViewBody9.tsx`) is a grid tile whose icon becomes a ring, for a quota or a usage: 96 px across on a page (`size-24`) and 80 px in a card (`size-20`), the track the named color's 14% tint (`hueTint`), the arc in the named color (`hue`) from the top clockwise with round ends, easing to a new ratio and snapping with Reduce Motion. The tile's value sits in the ring's centre in `text-xl` semibold tabular figures (`text-lg` in a card) with the unit under it in `text-submeta` `ink-3`; the title (`text-meta` medium) and subtitle (`text-submeta`) are centred below. The tile's surface, badge, press, switch and the grid's columns are a tile's as ever. A tile, not a new component, so the grid, its counting and badges are reused and an older app draws the same tile with the value and the percentage; a ring rather than the `progress` bar, because limits side by side read at a glance as shapes of one size with the amount in the middle, which bars cannot do in a tile's width. The color is the extension's named color (green while there is enough and amber once low is the expected use), never a hex. It reads out as "title: value unit, N%", a meter, or a button when the tile has an action.

An extension's graph (UI version 10's `graph`, #9727; `docs/extensions/extensions.md`, "UI version 10") is a canvas of nodes joined by edges, for work split into parts and plans whose steps wait for others; the same on the web, iOS and Android. Each node is one box of the graph node token's size (about 224 × 64 at the default text size, growing with the text, never with the content): a card surface (`surface` on the web, `.card()` on iOS, the group panel on Android) with the card radius and hairline border; the icon at the start in the quiet ink (the node's color when it has one); the title one line in the body's medium weight; the subtitle one line in the meta size and quiet ink; the badge as a chip at the end of the title line; progress as a 3-tall bar along the foot in the accent (the node's color when it has one). The node's `tone` (role color) or `color` (named color) is a 3-wide bar on its leading edge, never its text and never a fill. A selected node has a 2-wide accent ring. Edges are 1-wide curves in the hairline ink; a group's frame is a `Fill` tertiary rounded rectangle behind its members with its title above in the caption style. The fold control is a 24-high capsule (a circle for the chevron, widening for `+N`) on the children side (44 pt / 48 dp hit target): a chevron, or `+N` once folded. The canvas is the page's background with no grid; its floating buttons (Zoom out, Zoom in, Fit, Full screen) are the design system's icon buttons in one glass/surface group at the bottom end. Layout is in slots shared by every client (`spec/fixtures/graph-layout/`), so the same graph has the same shape everywhere; only the node pitch is each client's token. A canvas, not a longer list: a list shows the parts but not the shape, and a 300-part tree is a long scroll; native drawing, not an extension's picture or HTML, so nodes can be pressed, read aloud and drawn in every style. On the web, a page, Full screen and a card's Full screen additionally carry the control-room chrome around that canvas, while a compact card preview stays a still 200 px thumbnail: a collapsible outline of the visible nodes down the leading side, an in-place node inspector beside the canvas (facts, detail, links opened in the side panel), and a collapsible parallel wall-clock timeline beneath it, all in the design system's surfaces, rows and tokens. A wide run of sibling leaves packs several to a column instead of one long line, and a graph whose fit is below 65% opens at 75%, centred on the selected or first running, caution or danger node, so the opening view stays readable; Fit still goes down to 25%. On iOS, `Theme.Size.graphWheelZoomSensitivity` sets Command/Control+wheel zoom sensitivity; button-down pointer drags use the ordinary drag gesture.

The web task plan's workspace row (#3659) reuses the assistant form `fieldClass`, native labelled selects/inputs, and
the existing plan Edit dialog. The compact policy choice stays on the plan card because it authorizes where a task
will write; the optional branch and existing checkout picker are contextual fields. A server-backed, live-announced
path preview uses no browser path convention. Settings › Assistant reuses `Group`/`Row` and the manager-scoped
save-confirmation pattern for the default. Task details reuse the same read-only policy/path line. The workspace
controls and the plan Edit dialog load on demand with `lazyPage`, keeping the policy/path summary while the controls'
chunk arrives and the existing loading indicator for Edit. Neither editor adds controls to the cold first-screen
bundle. No new look,
theme, motion, primary navigation, or destructive action is introduced.

**Team panel** (web, #5180, after cindy's Team tab, `OrcaWorkerPanel`; `web/src/views/chat/TeamPanel.tsx`). It is a lead's side panel, opened from the **Team** button in the lead's header (Lucide's users-round, `users`). The button is shown at every width, with the attention dot while a worker needs you and the panel is shut. The panel has three parts:
- the side panel's header, titled Team, with "Workers: N · Running: M";
- a 32 px strip of the running workers in pane order. Each is a 24 px round pill, 11 px, with its `StatusMark` and title. The pill shown is filled `--ink` on `--surface`, as cindy's accent pill is; the others are `bg-field` with the hairline border. The strip scrolls sideways, with ‹ › and faded ends while there is more that way, and arrow keys, Home and End move between pills;
- the shown worker's own conversation, the chat page without its header.

With no worker running, the panel says "Waiting for worker". It opens and closes by itself with cindy's timing (`docs/web/web.md`, "Team"); by hand it opens at every width, and a narrow screen draws it over the page as every side panel. Left out from cindy, because Persona has no such actions on a herdr tab: closing the panel does not end the team, and there is no create, edit or archive on a pill and no dropdown layout.

**Side sessions panel** (web, #7944; `web/src/views/chat/TeamPanel.tsx` `SidesPanel`, `web/src/lib/side-panel.ts`). The Team panel's layout over a session's side sessions (3.4), opened from the **Side sessions** control in the parent's header: the `sideSessions` icon (Lucide's gallery-vertical-end, the web's match for SF Symbol `rectangle.stack`) and how many sides there are, one line in the header's end slot, 28 px tall, 12 px medium tabular figures, the hover fill and the open state as `IconButton`'s, with the attention dot (8 px, `bg-attention`) while a side needs you. It is shown only while the session has sides, and the chat's ⋯ has the same thing as "Side sessions (N)". In zh-Hans the feature is 分支会话 (the owner's word, 2026-11-15), in ja 子セッション; neither is 子任务 / 子 Agent / サブタスク, which name child Agents and tasks. The panel has:
- the side panel's header, titled Side sessions, with the shown side's title under it;
- the Team panel's strip, with the sides flat (sides of sides too): needs you first, then working, then by last activity, newest first, stopped ones last. Each pill is the `StatusMark` and the session's title; its tooltip and spoken name add the plain state word. No ids, paths, engine or model names;
- the shown side's own conversation, the chat page without its header (its composer, its cards and Stop), landed at the message a link asked for.

It never opens by itself: by hand, or for a link to a side (a deep link, a notification, a card in Needs you or the Inbox, a search hit), which opens the parent with that side shown, at any width. With no side left it says "No side sessions". There is no action that starts a side session (the desktop app makes them). A session with both workers and sides shows both controls, each with its own panel. Why the Team panel's layout and not a new one: it is already the web's way to read and answer another session from a session's page, and two layouts for the same job would differ in their keys and their ends (principle 4).

The closeout card (web, #3612; `web/src/views/chat/CloseoutCard.tsx`) is a stopped Agent session's own account of its work, drawn above the transcript and never inside a message: the card surface (`rounded-card`, `bg-surface`, `shadow-card`) at the chat column's width, a head that folds the details (`Collapse`), and label / value rows. It says it is the Agent's own report. A claim the hub checked shows the finding beside it in one of three states that differ in words, mark and color: confirmed (`--green` with the check), contradicted (`--orange` on `--orange-tint` with the warning mark) and not checked (`--ink-3`). Why a new card: no existing surface sets a claim against a finding, and a status tint over the whole card would read as an error, which a closeout is not (`docs/web/web.md`, "Color hierarchy and quiet surfaces"). Other clients that draw it keep the three states and the "own report" line. iOS (`apple/Persona/Views/Chat/CloseoutCard.swift`) draws it in the chat's folding header on the plain well fill the goal bar and the workflow card use there (glass does not fade with the header), with `Theme.positive` and the check for confirmed, the caution `Chip` with the warning mark for contradicted, and secondary words with a dash for not checked. Its head is one button the height of a tap target at every text size (`tapTarget`, section 6.3), the card's own top and bottom padding inside it. Verified commands named by inline code or recognizable command text add factual command evidence beside the claim, matched only against this closeout exchange's normalized tool calls and results. The evidence shows the actual recorded command and, as recorded, a numeric exit, `is_error`, background or running state, no match, or unavailable evidence; `is_error: false` is not an exit 0 and no match never means the command definitely did not run. Folded records load on demand through the transcript's existing fold reader; a failed or capped read stays unavailable with Retry, not not-found. This adds no green verification badge, model judgment or approval.

The answerable decision row (iOS, #3804; `apple/Persona/Views/Assistant/Tasks/DecisionRow.swift`, after the web's `views/assistant/DecisionRow.tsx`) answers a decision where the app lists what needs the owner: the Needs You tray and the task board. It adds no surface: it takes the place of the one-button row (`BriefAcceptRow`) in the tray or board row that held it; in the tray, where a decision row is folded until opened (#5569), it leaves out the title the folded line already says and keeps Later and Don't handle under its options, as `BriefAcceptRow` does there. Text first: the task's title, the question, the hub's background (two lines at most), each option with what it leads to (`RecommendedTag` on the first, under its label when the two do not fit on one line), and what happens when nobody answers with what waiting costs. Then one line of buttons, each a tap target tall as `BriefAcceptRow`'s are (#5569): the options as `.themedButton` in the hub's order, the recommended one the only prominent one (section 3.7), **Other…** (opens the task, to answer in words) and an icon-only **⋯** `Menu` (`Theme.Icon.more` on a label of the buttons' text height, `docs/lessons/ios.md` "An icon-only themed button takes its glyph's size (2026-11-07, #58)"), which holds the options that do not fit, the hub's fixed answers each with what it does, and Complete Directly. Labels never wrap (section 1.2 rule 3): `ViewThatFits` moves options into the menu from the end; the first stays and, alone and still too wide, cuts its label while **Other…** and **⋯** keep theirs; at the accessibility sizes the group is one column. Two-tap arming: an option of an `irreversible` or `visible_to_others` decision takes two taps; the first relabels it "Confirm: …" (VoiceOver value "Tap again to confirm"), and the armed option keeps a button even when its longer label leaves room for fewer: it takes the last place, so with room for one option it stands alone and the recommended one waits in the menu; tapping another option arms that one instead. One request at a time: an answer or Complete Directly spins its control and disables the whole row until the hub replies. A fixed answer leaves the row answerable with "Sent back to Persona" above the buttons; an option puts "Answered: …" in the buttons' place; a 409 "Already handled"; any other failure is a toast, and the buttons (and an armed option) stay as they were. Why a new row and not more buttons on `BriefAcceptRow`: a decision's options are the answer itself and need what each leads to beside them. A decision without options keeps `BriefAcceptRow`.

`OrphanedLine` (iOS, #3618; `apple/Persona/Views/Assistant/Tasks/SupervisionViews.swift`) is one caption line, "Nobody is working on this · <reason>", in `Theme.cautionText` with the warning mark (`Theme.Icon.error`) under a task's state on the To-dos row and the to-do's page; the task board puts the same words first in its subtitle. The words carry the state, so color is not the only signal (section 6.3). It is not shown while the task waits for the user, nor on a done to-do. It is a status line, not a notice: no surface and no action of its own. The frozen Mac app draws neither the decision row nor this line.

The brief box (web and Android, #3822; `web/src/views/assistant/TaskBrief.tsx`, `android/feature/chat/src/main/kotlin/persona/feature/chat/PlanTaskBrief.kt`) shows what a task is held to. The web draws it on the plan card before **Start** and on the to-do's page above its criteria; Android draws it on the open plan card before **Start**. It adds no surface: it is the inset well the review checklist already uses (`rounded-control`, `bg-inset`, `shadow-hairline`), with the closeout card's label / value rows. Whose words a line holds is never left to styling: the owner's own are introduced as theirs with the date, work they did not ask for names who started it, and a brief made later says it was reconstructed and labels the hand-over prompt as that. Long text is held to a few lines with **Show all** in place (principle 2). On the plan card the whole brief is folded to who asked and one line that counts the rest, behind that one control, and the card's buttons stay at the bottom edge of the tray it scrolls in: added detail never puts a pending request's answer out of reach (section 4.8). The criteria keep one list, the review checklist's own (met, missing, open, not checked); under it a finished task adds only the criteria a scope change dropped (`--ink-3`, the undo mark, the criterion struck through, the word and the change that dropped it) and what was done beyond the brief. Overdue is a caution chip (`--orange` on `--orange-tint` with the clock) plus a sentence, not a tint over the box. Other clients that draw a brief keep the same first lines and add the dropped state to their one list of criteria.

**Background tasks panel** (web, #5421, the owner's requested alignment with cindy's Background tasks tab, `BackgroundTasksBody`; `web/src/views/chat/BackgroundPanel.tsx`). An Agent's side panel, opened only by hand (the top bar's button, the loop mark, a workflow card, `/workflows`). Under the side panel's header (Background tasks, list mark):
- sections **Running** and **Completed**, 11 px medium headings in `--ink-3`;
- rows of a 20 px round kind mark on the chip fill, a 13 px status mark and the 13 px title, with an 11 px meta line ("status · n/m Agent · duration · tokens · tool uses"); only a failure is red; only rows that open something are buttons;
- the loop as the first Running row (Persona's own; cindy lists none), its Stop confirmed inline;
- a run's detail: Back, the run's mark and name, then the run's card;
- a command's title, status and mono command line share one rounded hover / focus target (#9169). The independent Copy command icon appears on hover, keyboard focus and touch, without opening the output;
- a command's detail opens inside the same panel (`BackgroundCommandDetail.tsx`), reusing the run's Back bar and existing Button / IconButton controls. Status, runtime and start time are label / value rows. Running time updates; an ended command shows its retained result, known exit code and fixed runtime. The full command is wrapped monospace, folded to three lines with **Show full command**; its Copy command icon sits beside it. Output has a label and line count, follows new text until scrolled up, and retains the last good text with Retry on a failed read. An open detail follows the command into Completed. On an older hub / node without retained outcomes, removal from Running says **Not listed as running**, without an inferred outcome or a still-ticking clock. A shell command's or Monitor's row opens this detail even before the hub knows its output file (a Monitor's arrives with its first event, #10852): the rows and command are the same, Output says **No output yet** (**No output** once it ended) and there is no **Open as file**; when the file arrives the detail reads it in place, keeping the focus and the command's fold. A subagent's row opens nothing without an output file, since its work is its own conversation. For a detail opened from the Background tasks list, Back restores list scroll and row focus, and Escape returns to the list first, then closes, except while a menu, dialog or IME owns it. For one opened from Session overview, Back or Escape closes the panel and restores the mounted overview, its scroll and the originating row's focus. **Open as file** remains a secondary action. Why a command-specific detail: the file viewer does not follow running output or preserve the Background tasks list; a generic panel-history stack is unnecessary here.

The status words are the task cards' (`taskStatusWord`), and the count on the top bar's button is the running background tasks and runs (`backgroundCount`), the same number the panel's Running section shows without the loop.

**Android Background Tasks** (#5432). The Agent chat menu and Glass activity pill lead to the same native
full-screen overlay. The root uses a leading Close action. A detail uses Back and returns focus to its row.
The composer offers a local `/workflows` command, and an inline workflow card can open its run. Running and
Completed stay separate in the same list. The loop keeps its existing confirmation. The workflow detail uses
`JWorkflowCard`, and command details use the existing typography, copy action, notice, and file viewer.
An outcome that an older hub did not retain says **Not listed as running**. The output preview follows the tail
when the reader stays at the end and retains the last good result on an error. The requirements and file scope
are in `docs/android/android.md` ("Background tasks in Agent chat").

**Hand-off cards** (web, #5096; the owner asked that this part of the main chat follow cindy's `SessionHandoffCard` exactly, which replaces the "Dispatched: <title> · Open" line). iOS draws the same shape (`DispatchHandoffCard`): a well with ↳, title and badge, "3 min. ago · state", ↗, the failure line in `Theme.danger`. Each Agent a dispatch started or a task took over is one full-width card in the chat (`views/assistant/Handoff.tsx`; iOS and Android, #5110):
- **Frame:** a bordered `rounded-xl` surface, 16 px padding and 14 px gaps (12 / 12 at phone width), whose border turns `--ink` on hover.
- **Mark:** a 36 px round `--field` well with the ↳ mark (`cornerDownRight`), which inverts on hover.
- **Title line:** the title (14 px semibold, one line) and a 10 px uppercase pill badge: New, Resumed and Continuation are filled `--ink`, Already active is outlined. On iOS and web, a plan that carries on an existing to-do says Continuation, including when its Agent fails to start. Resumed means the same Agent was reopened.
- **Second line** (12 px): the history mark, "last active …" (the time alone at phone width), then ` · ` and the Agent's state from `StatusMark` and `lib/state-labels.ts`, in the attention color when it needs the user.
- **End:** ↗ (`arrowUpRight`).
- **Open:** the card opens the Agent where Settings › Agent preferences › "How Agents open" says (#5424): the side panel beside the chat (the default; over the chat on a phone-sized window), or the Agent's page. The card looks the same either way. Before an Agent is up, the card has no ↗ unless the task can be opened, and its second line says the Agent is starting, why it did not start (`--red`), or that the task closed or is gone. On iOS and web, a failure with no readable cause says "Didn't start." and offers details only when the card can open them. If the card cannot open, its failure line wraps so the explanation can be read on the card.

Two departures from cindy, both from Persona's rules:
- cindy shows a short session id on the card. The Assistant's chat keeps ids out of sight (super rule 4, section 5.6), so here it is only in the card's tooltip.
- The state word is added, so an Agent that needs the user is visible without opening anything (section 3.4).

Set to Full page, the Agent's page opened from a card shows cindy's way-back strip above the chat (`views/chat/HandoffBack.tsx`): a 40 px `rounded-xl` bordered row at the chat column's width, ↰ (`cornerUpLeft`) and "Came from <conversation> · click to return", with ✕ to put it away. Returning goes back in the history, and the chat opens where it was left (`lib/handoff.ts`).

No new token. The two icons are added to `ui/icons.tsx` in the existing stroke grammar. iOS and Android draw these cards in the chat, one per Agent, and a card with an Agent opens that Agent's page (#5110).

**Changed-files card** (web, #5157, `web/src/views/chat/TurnChanges.tsx`; cindy's `TurnChangesCard`, at the owner's request, 2026-11-12), Agents only (super rules 0 and 4): the last thing in a finished Agent exchange that edited files.
- **Frame:** a `rounded-xl` `bg-surface` section with a 1 px `--line` border and 4 px above and below.
- **Head** (at least 64 px, 16 px across, 12 px down, 12 px gaps): a 36 px `--field` tile (`rounded-control`) with the 18 px `fileDiff` mark in `--ink-2`; **Changed N files** (14 px medium `--ink`, one line) with `+N -N` under it (12 px mono, `--diff-add-fg` / `--diff-del-fg`, ASCII minus); **Review**, the standard secondary `Button`, on the right.
- **Files** (under a `--line` hairline, 12 px across, 8 px down): 36 px rows (44 on a touch screen), `rounded-control`, `hover:bg-hover`: the 15 px `file` mark in `--ink-2`, the path relative to the session's folder (13 px `--ink`, truncated, the full path as its title) and its own `+N -N` (each only above zero). Three rows, then **Show N more files** ⌄ / **Show less** ⌃ (13 px `--ink-2`, 32 px, hover `--ink`).
- **Counts** are the exchange's own successful edits, the numbers each edit's row shows, counted without drawing a diff; folded work counts by the changes its fold carries, so a reload keeps the card. A fold from an older node carries none, and an exchange whose start is not on the page is missing edits: neither has a card (principle 5: no count that cannot be known).
- **Review** opens the branch's review (commits and uncommitted work); a file row goes to its file there, or opens the file itself when the review does not list it or the file is outside the session's folder.

Deviations from cindy: no Undo / Reapply (Persona cannot apply or revert a turn's patch), the plain `file` mark instead of file-type icons, rows a finger tall on touch screens (6.3), and 评审 for Review in Chinese (the Review panel's word). The `fileDiff` and `alertCircle` marks are added to `ui/icons.tsx` (Lucide's file-diff and circle-alert, as cindy's); no new token.

#### Connected Inbox message cards (#856)

Lark CLI Inbox items with a source AppLink offer **Open in Feishu** (or **Open in Lark**)
while **AI draft reply** remains available. This secondary Button opens the source client at the original
message or topic using the CLI's unmodified link. Missing links have no opening action;
the existing in-app Original action remains available.
For older items without a stored link, the loaded original message supplies the
same action next to the reply-target details; a stored header action is never duplicated.

The web Inbox uses the existing connector conversation for source-provided messages;
legacy collected text is never split into inferred messages. A supported card uses
`bg-surface`, `rounded-card`, the existing stroke icons and the Markdown/link reader.
Its linked subject and description come first; author, repository and original text
are disclosed in details. Long descriptions have a preview, reversible expansion
and a bottom collapse action that restores focus to the expansion control.
Reply targets show a friendly name and a source-backed preview when available;
opaque routing IDs and read diagnostics stay in details. The layout changes no
sending identity or confirmation behavior.

The web Agent conversation header's **Session overview** list button toggles a panel from the right side of the toolbar (`web/src/views/chat/ChatView.tsx`, `web/src/views/chat/SessionOverview.tsx`). The panel uses the app's surface and keeps its own scroll. It sits beside the conversation when there is room and above the transcript in a narrow conversation. The reader can keep it open while working in the chat. The environment and context sections use the current session's metadata. The subagent row shows running and ended counts and opens the tree on demand. Background activity uses the Background tasks panel's list rows, workflow details, output viewer and loop controls. A detail opened from the overview hides the mounted overview instead of squeezing it; Back or Escape closes the detail and restores the overview's scroll and originating row's focus. While the overview stays open, Cmd+B can close and reopen its detail. Turning the overview off or leaving its conversation closes the dependent detail and removes it from Cmd+B memory, so a later toggle cannot expose a stale origin. List-origin details keep ordinary Cmd+B memory. The finished task disclosure counts distinct terminal notices in the loaded transcript. The overview and its disclosures start closed.

### 3.4 Lists and rows

- Web History (#10240) is an Agents destination for non-running sessions, grouped by local activity day. It shares folded sidebar state and visibility before filtering; day counts equal the displayed filtered rows, including only loaded content-search matches. The primary switch stays Assistant / Agents. Agents + owns New Agent, New terminal and Open terminals; History never starts on a remembered terminal inventory (details in `docs/web/web.md`).

- **Native workspace pins** (#9300): the Agents workspace tree shows a host-pinned workspace with the existing pin mark,
  ordered before its unpinned siblings (newest pin first, then the existing stable order). Its repository moves ahead
  of unpinned repositories. Match the exact machine/source/workspace identity, never the folder or project name; empty
  pinned workspaces and their unloaded, unarchived conversations remain available. Search and state filters still apply
  and explicit folds remain under the user's control. The source app changes these pins; Persona's session/project
  preferences and Assistant directions stay independent (`docs/integrations/paseo.md`, "Pinned workspaces").

- Tiles (an extension's `grid` and `carousel`, `ExtensionTiles.swift`, #827): `Theme.Radius.card` on the chip fill, `Theme.Size.tileMin` / `tileHeight` / `tileNarrow` / `tileRegular`; one that is on takes its named color's tint and a ring of it, the words never take the color.
  A carousel scrolls to its section's edges and is cut there. On a page's list its row spans the section, its first tile at `Space.page`; inside a card (a section of `ExtensionViewStack`, a code page with tabs, a columns card) it takes the card's width, its first tile at `CardStyle.padding` in line with the card's other content, cut at the card's edges and never drawn over them (#10233).

- Page chrome: `.themedPage()`, `.themedGroupedPage()`, `.themedRow()` (`apple/Persona/DesignSystem/Styling.swift:13-22`). Settings uses `SettingsForm` and `SettingsRow`.
- A row without a background that reaches the edges of an inset-grouped cell (`listRowInsets` leading 0 and `listRowBackground(Color.clear)`, such as the Knowledge kind bar) adds `.unclippedListRow()` (`apple/Persona/DesignSystem/UnclippedListRow.swift`): the cell's 26 pt rounded corners clip it otherwise, and its first chips lose a corner (#7735). A row with a background keeps the clipping, or its square corners show.
- Row height at least 44. Sidebar rows use `Theme.Sidebar`. The iOS Agents list (#3753): `Theme.Size.tapTarget` is its least row height in every grouping and display; a row that is one plain button takes taps on its label alone, so the row gives its room above and below to the label (`ButtonRowInsets`, `ButtonRowLabel` in `apple/Persona/Views/Chats/RepoTreeRows.swift`) and the label is the whole row; folding headings are `TouchHeight` tall and fold chevrons `tapTargetFrame` squares. The Mac keeps its sizes.
- A list read further down hears of news without being moved (Android `JLatestPill`, `android/core/designsystem/src/main/kotlin/persona/core/designsystem/LatestPill.kt`; iOS keeps the row being read in place and holds the list while it scrolls, `ReaderAnchor` / `Hold`): what needs the user or went down is given to its row at once, even while the list scrolls (principle 4: what needs the user is visible without opening anything), the order and the rest of the page wait for the list to rest, at most a bounded time. What came out of view, counted as a tap on the pill shows it, is one line in a pill at the list's edge, away from the top only. The pill never scrolls the list, takes focus or waits for the hold, and does not stand in for the row's own state; a tap goes to the latest. Its words are one line and spoken as written; the screen reader says what the tap does.
- Chip family: `Chip` (label), `FilterChip` (toggle), `LinkBadge`, `CountBadge`, plus 50+ lookalikes in feature code (DS-21). A compact `Chip` draws its symbol before its words (#6578; before, it dropped it); one that is a button in a row (the minutes' time, a to-do's Got it) takes a 44 pt hit area outside what it draws.
- Status: `StatusDot` (fixed size 9, labelled), `AgentStateBadge`.
- Row actions: swipe actions and a context menu with the same actions. Agents rows on iOS have no swipe (context menu only, #1244).
  - The Inbox's rows (iOS `InboxView`, Android `InboxItemRow` and `InboxConversationItemRow`, #4210 T1): one trailing swipe, from the end, and nothing from the start. It is **Done** in the positive color, or **Move Back** with the undo arrow (`Theme.Icon.undo`) in the accent color on a done item, so the rows of the Done section move back with the same swipe; there is none while a write for the row is out or items are being picked. The hand-off to the assistant is not a swipe: it is in the row's long-press menu (on Android, where a long press on an open row starts a selection, in the picked row's More). Android's swipe component (`JSwipeActions`) takes a tone (`SwipeTone`, accent or positive), which Glass draws as a fill of that color under the row, as iOS tints the action; Brief and Board keep their neutral backing and the action's status color. Every action of a swipe is also a custom accessibility action on the row (`swipeActionSemantics`, which `MessageItem` applies), so TalkBack and switch access reach it without swiping.
  - The assistant's conversation history (iOS `AssistantSidebar`): a row swipes left to Archive (not the main conversation); its long press shows what the conversation last said and when above the menu, whose items are Pin, Rename…, Archive, Copy Session ID, Copy Diagnostics and End…; "Archived (N)" ends the list while there are any and opens them, where a tap opens one and Unarchive brings it back. Android shows the preview in the action sheet (`JActionSheet`'s `preview` and `previewCaption`, six lines at most), and the archived rows keep a visible Unarchive as its Agents archive does.
- Rows with a control (extension pages, UI version 6, iOS `ExtensionControlRow`): a system switch or stepper at the row's end, a slider on its own line under the row, labeled by the row's title. A row that also opens something keeps its words tappable and the control separate. A confirm is an alert whose Cancel puts the control back (section 4.4).
- **A switch that mirrors another device** (#4741, the Recordings page's Today, `RecordingDayTodaySection`): the All-Day Recording switch is a `SettingsSwitchRow` whose value is the Apple Watch's reported state (on while it records or is paused, `RecordingDayWatchState.isOn`; off when it is off or stopped by itself, from low battery or storage, a failed write, an audio reset or the app ending, though a stopped day still continues with a tap on the watch and its line says "Stopped — tap Continue on your Apple Watch"), never the tap alone. Flipping it sends the watch a command. Until the watch's state answers it, for at most 2 minutes, the switch holds the asked value and its line says the command is on its way ("Tap Start on your Apple Watch to begin", "Stopping on your Apple Watch…"); after that the switch shows the watch's state again and the line says "Your Apple Watch hasn't answered. Open Persona on it." for 10 more minutes (`RecordingDayStatus.line`, `RecordingDaySwitch`). The line under the title always says where the action happens ("Paused — tap Continue on your Apple Watch"). Turning it on before the user agreed opens the consent sheet (3.6) instead, and the start goes once they agree. Why: the phone can't open the watch's microphone; a switch that flipped on the tap would claim a recording that hasn't started.
- **Recap pages reuse the minutes' layout** (#4741, a recorded day's page, `RecordingDayView`): a compact header, then the recap drawn as a meeting's minutes are (`RecordingMinutesSections`, `docs/apple/recording.md` "Minutes"), then one last section: a **Transcript** link to the day's transcript on its own page (`RecordingDayTranscriptView`, by the hour), with the actions under it (**Erase Last 10 Minutes** on today's day while it is open, **Delete Recording**). A meeting's page instead shows its transcript inline, in a section of its own above its actions. The recap: summary with Copy and Share; the timeline as sections named for the time of day (Morning, Afternoon, Evening) with a meeting entry marked by `Theme.Icon.meetingMark` and its name; points and suggested to-dos as the minutes' items (a mark, the words, `RecordingMinutes.byline`, a time chip: `Chip` `.tint(Theme.accent)`, compact, with a 44 pt hit area, opening the transcript at that line and lighting it for 2 s). Before it exists the section says what it waits for, "Writing the recap…" with a spinner, or the failure with Retry, as minutes do. Suggested to-dos are read-only here, with a footer saying they are confirmed on the web.
- Web rows: 32 px desktop, 44 px touch. Android rows: min 56 dp.
- Web sidebar hierarchy (#7609): peer section titles such as **Persona chats** and **Agents** start at the same inset. They also stand evenly apart (#10637): each top-level heading's title row is one box, the `sidebar-heading` utility (`--sidebar-heading`, 2.5), with the controls beside its title centred in it and never setting its height, so Pages with only a count, Persona chats with New chat and Agents with its counts, fold all, Filter, ⋯ and + are equally tall, folded or not (`Section`'s `top`; the Agents list's title row, complete and paged). A group's heading inside a list keeps its own row. In a nested list, use the shared `sidebar-indent` utility (`--sidebar-indent`, 12 px) once at each parent boundary. In Persona chats, **Pinned**, **Recent**, and **All Persona chats** sit one step in. Conversation rows and Show more sit another step in under their subgroup. The same unit indents children in the sidebar's nested trees. Indent the content wrapper so the right edge stays available for counts and menus. A fold is a button with a chevron and `aria-expanded`. A destination uses a link with `aria-current` when selected. Keep help beside the control in an InfoTip when the scope or count needs explanation.
- Web sidebar groups share visible disclosure chevrons and sticky, interactive headings in every grouping (to-do, computer, repository, project and state). Nested group headings stack below their parents and the measured Persona row; the next sibling replaces the current heading as it scrolls past. The Agents title, grouping segments and quick filters form one sticky toolbar below Persona (at the viewport top on the phone Agents page). One chevron button (`GroupFoldButton`, drawn by `FoldAllButton` in `web/src/ui/FoldAllButton.tsx`) is the list's only Collapse all / Expand all; the list menu does not repeat them, and the keyboard shortcut stays (#6447). Its icon shows the next action like its label: corners pointing in (`foldToggle`) for Collapse all, pointing out (`unfoldToggle`) for Expand all, so a folded list's button reads as "open". It acts on the list under the controls only: the Agents title, grouping segments, quick filters, search and Filter never fold with it. Its action follows what is on screen, so a click always changes something (`agentsFoldAction`): Collapse all while any visible group is open folds every group to its heading, and Expand all when they are all folded opens them; with the list folded under the Agents title (the title's own click), Expand all opens the list and every group in it. The chat-list view, and a list with no groups at all (a grouping still loading, nothing listed), have no headings to fold to, so the button is not shown: folding a flat list's rows would leave a blank under the search that reads as an empty list, and a flat list gets no invented heading (below). A first fix folded the whole block there, taking the controls with it; the owner rejected it. Its spoken label names the next action, and the same words show in a tooltip under it on hover after 300 ms and on keyboard focus (a dark tooltip in the `--tooltip-*` colors, no native `title`), with no second line explaining them: the owner found Collapse all / Expand all needs none. Why: before #6447 it acted on the groups alone, so in the chat-list view and under a folded block a click only flipped its own label. The toolbar stays above the section headings on an opaque theme background, with no fog over the first heading. A heading pinned over its group's rows, the innermost one only, draws the Persona row's divider under it (a 1 px `--line` hairline at the same inset), and no heading does at rest: a row half under a pinned heading on the same background, with no edge, read as text run into it (audit item 51 of the 2026-11-14 web usability audit). A hairline, not a fade, for the same reason the toolbar has no fog. Individual folds remain independently reversible; flat conversation lists have no invented group headings.
- A grouping's top level is one kind of row (#6458): in the web's **By to-do** that is the to-do's row (chevron, mark, title, count, state word, each in the same place), and the groups beside the to-dos (Not on a to-do, Recent (not running)) are rows of that kind, never a heading of another look. Where an Agent came from is its row's source mark (Terminal, Lark, Desktop app), not a group of its own: the Agents opened by hand in a terminal are in Not on a to-do (#6591). A group's state is the most urgent of its sessions', after its count, as a to-do's is; a to-do's word is the one its own page uses (5.6). No grouping nests a location (repository, folder) under a grouping that is not by location. What a group holds, when its name does not say, is an ⓘ beside its title (3.8 "Info tips") and the group's empty state, never a line always shown under it. Rows that fold groups select no text (`select-none`): a drag over them painted the grey `::selection` behind each line.
- A long list of records on a detail page (a to-do's Decisions, web and iOS, #6447): the first five show, with **Show all N** under them and **Show the first 5** to go back. The header carries the fold-all button with the Agents title row's meaning (the web's chevron icon, corners in to collapse and out to expand, and its name as the tooltip; iOS's Collapse All / Expand All symbols, `Theme.Icon.collapseAll` / `expandAll`): Collapse all while any record is shown folds the list away, Expand all shows it whole. The header's count is unchanged by what is shown. Nothing is stored: each to-do opens at the first five, and recording or revising one shows the whole list so the new one is in view. No button while the list is empty.
- **Inbox matter detail** (#6611; web `InboxDetail.tsx` and `InboxEvent.tsx`): the title's small line says where and who, then the current state, any suggestion, the background and the timeline. The state sentence keeps the processing-conversation link and offers **Say something** when no suggestion is waiting, or **Try again** after a failure; the instruction field reuses the suggestion editor's input and standard small buttons. The timeline starts with the three newest steps and expands on request. A `reported` step always uses the reported icon and is distinguished from program-confirmed steps by the existing neutral **Tag** ("Assistant says") and secondary text, not a new surface, tint or token. Source and Agent states reuse the existing brand marks and shared state words; missing fields keep the old presentation.
  - On iOS (`InboxEventViews.swift`), the header uses `InboxNow.meta` when `where` exists and the old source line otherwise. `InboxNow.text(for:)` supplies the current sentence for every known state, including done states and states with a suggestion; a state key alone opens the event detail. Row and detail both render `InboxNow.symbol(for:)` through `InboxNowSymbol`: waiting has a still clock, only queued / looking / agent_working spin, Needs-you states use the existing mark (failed is red), and done / FYI / folded have no mark. The same state row carries the instruction action; the input uses the suggestion editor's well and `InboxSuggestionButton`, and a stale answer or error uses `InboxSuggestionModel`'s existing notice and error. The timeline's three-step projection and `via` icon meaning are shared, fixture-checked logic. A source id draws its `BrandMark` when one exists, otherwise its generic `ThemedIcon`; `reported` uses `Theme.Icon.reported`, a neutral compact `Chip` ("Assistant says") and secondary text; the row shows `detail` when present, otherwise `text`, so the server's older-app prefix is not repeated; inline detail is not repeated in an expansion, and detail alone offers no expander. Agent and Agent-stopped rows with a session link read that one session's `AgentState.label` and `tone`; an unlisted session shows no state. Three or fewer entries have no expander.
- Status sections (web To-dos, #3156): a list grouped by what the user should do next uses folding section titles in a fixed order (what the user has to act on, then the unfinished work, then what just finished; only Shelved and Done start folded, Done last, #5707), one row per piece of work with related items behind an "N related" disclosure, and stale items folded under Shelved with a one-tap Restore instead of being hidden or deleted. On the web, **Parked** is a peer of Needs you and In progress (#9325): it combines explicit external holds with review/merge waits backed only by fresh matching PR facts. The tray, To-dos status list and All board use the same wait classification, one task and one open-count contribution even with multiple PRs or Agents. PR waits keep their links and concise review/check/conflict blockers. A parked tray row uses a neutral clock, not a finished-turn check; an elapsed fallback stays visible until the hub changes its state. The Parked entry appears while it has rows or its tray remains open. Selecting an entry reveals its whole capsule and count in the horizontal scroller, including at the largest text size. Why one group: both are waiting rather than active work; the existing PR predicate and rule still distinguish merge waits from unrelated holds. A task waiting for its supervisor's review has an entry of its own there too, **Persona reviewing** (#8678), shown only while one waits and drawn neutral (grey `ink-3` dot, `ink-2` label, no attention or working colour), because nothing in it is the user's to do; Needs you keeps only what is. A row that is only worth a look (Just finished) has a one-tap **Got it** at the end of its status line, under the time (#6578, iOS #7148; Android: the end of its last line), which files it under Done at once with an Undo toast (4.4, reversible rung); opening the item counts as seeing it, without a word. It is a small capsule, never a line of its own: the row is exactly as tall as without it and the title keeps its room. The web draws it 20 px tall with the check and the word in `--ink-2` on `bg-surface` with `shadow-btn` (hover `bg-hover`), hanging over the 18 px line by negative margins, a 44 px finger target through `touch-hit`; iOS draws it as a compact accent `Chip` with the check, its capsule hanging over the line by `Chip.compactInset`, hit like a row through `tapTargetOutset()`, and keeps the leading swipe action after the full swipe's own. They share one line through xxxLarge, the interface-row text cap (#6813), when they fit whole; on a narrow row with a long status, `ViewThatFits` puts only Got It at the right of the next line, and `MeasuredOnce` keeps row movement from remeasuring the candidates. Its press stays its own: it never opens the item. Why not under the words: a button line made these rows taller than every other and left the room under the time empty. Rules in `docs/assistant/todo-status-sop.md` ("Seen"). Why not the folder groups: they say where work lives, not who has to act. Why unfinished work above Just finished: what still has to be done should not take a scroll and a tap while finished work sits above it.
  - Parked wait metadata (web): render one shared, lazy `TaskWaitInfo` through `ParkedTaskRow` in the Assistant Parked tray, To-dos, All board and task details. The caller still decides membership and preserves owner-decision/live-work precedence; the row adds no grouping or status rule. At narrow widths, wrap the wait target, age, valid last contact and wake on separate readable lines; keep the full request, reason and deadline in expandable details. Say a sent request is not recorded only where one could be (person reply, review/merge, legacy and implicit PR waits); an external event or a date never carries one, so it has no request line (#10527). When external-event evidence carries a hub-verified `interaction_id`, show that exact pending request once through the existing Needs you / `InteractionCard` flow, whether its worker is active, idle or released; an already-associated active card is not duplicated. The interaction's own resolve and explicit defer actions, schedule and risk stay authoritative. A worker shared by several to-dos keeps the existing one-home routing: the session's tied to-do wins, else the first listing; non-owners stay Parked. Once the exact card resolves, its stale `work.interaction_id` session state does not create a synthetic ask, while a different current card remains actionable. A missing, resolved or wrong-worker ID stays an ordinary Parked wait with its original fallback, and target text or a source URL never becomes an owner request. Known wait starts sort oldest first, unknown starts last, with a stable to-do-ID tie-break. A valid sent-contact start reads **Waiting**; a review-time fallback reads **Parked**. A due label is informational: `until` wakes the supervisor, while only an aged valid contact enables the editable local reminder draft; neither asks the owner to act or says work resumed. Copy is local, never a send. Successful copy dismisses the editor; failed copy and text edited during clipboard permission remain recoverable. Display projection: `web/src/lib/task-wait-info.ts`; validation/order helpers: `web/src/lib/task-wait.ts`; UI: `web/src/views/assistant/TaskWaitInfo.tsx`, `web/src/views/assistant/ParkedTaskRow.tsx`.
  - Parked source names (web, #9730): use compact object names (PR/issue number, comment on that number, or matching recorded Lark topic/channel), never numbered source placeholders. Preserve the full source URL for inspection. Explicit wait evidence takes precedence over task PR associations; only a wait without explicit evidence falls back to all eligible merge PRs. Match topic metadata to the source chat/thread before naming it, do not invent a missing name, and render no source for an unusable URL. Use the existing link layout, colors and touch targets; naming adds no fetch or task-state write.
  - Section titles are the Agents list's state headers (`SectionHeader`: the name and a `CountBadge` in the state's tone; `FoldButton` for the folding ones), and the count is the items listed under the title. No separate summary line repeats them (#3712).
  - On the V1 To-dos list (below; iOS, hubs with `todo_view`, from #10465's switch-on) there is no Got it: opening a to-do counts as seeing it; Shelved becomes the Inactive mark inside To do.
  - "N related" sits inside the row it belongs to, under its words; opened, the related rows come under it a level in, with a line down their side.
  - A filter and a sort for such a list go in one button's menu next to the view switch (on the web, next to the search field), the active filter also as a chip that clears it ("List controls" below); the sort orders rows inside each section and never moves a section (To-dos on iOS, Android and the web, #3740).
- **To-dos list** (#10465; iOS, hubs with `todo_view`, from #10465's switch-on; `docs/design/proposals/todos-redesign.md`
  3.3 and 3.4, the V1 frames `iphone/list`): the hub files each to-do (its `view`'s section) and the list draws the
  sections in the hub's order, what the owner has to act on first: Needs you, with Can wait folded at its foot (listed,
  not counted); Working; Waiting; To do; Just finished; then Someday folded, and under All the to-dos closed today folded
  as Done today. A row without a view (an older manager's) is filed by the app's frozen rules. Inside a section the rows
  follow the hub's rank, then the assistant, then the id; ⋯ › Sort By orders them otherwise and never moves a section,
  and its Show Someday and Show Done Today open those folds.
  - **The section bar** is the Inbox's ("List controls" below; `CapsuleFilterBar`, the grouping menu at its end under a
    Group By heading: Phase, Project, Due Date, with Needs you a section at the top in each), with To-dos' differences:
    it lands on All, its first choice, each time and keeps no choice; To do takes FYI's place; In Progress (Working and
    Waiting, with Just finished listed at its foot and not counted) and To Do keep their neutral counts at every width,
    the strip scrolling rather than dropping them; Done shows only while it has rows. Needs You carries the blue N and
    always shows. Its choices are buttons, so in Title Case with the Inbox's words (All · Needs You · In Progress · To
    Do · Done; section 5), while the section headings under it are in sentence case (Needs you, To do). On iOS 26 it is
    pinned over the list, which scrolls under it behind the scroll edge's effect; iOS 18 has no such effect to draw
    behind a pinned bar, so there it is the list's first row, as the Inbox's.
  - **⋯ › Sort By** offers Suggested (推荐顺序, the hub's order inside each section, the default), Recent Activity, Due
    Date and Priority; Suggested is a sort's name only, apart from suggested steps (建议).
  - **The calm rule**: while only the owner's plain To do rows exist (Someday and Done do not count) the bar and the
    section headers are not drawn, so the list reads as a plain list; they come back once two sections have rows.
  - **The row** is the work-item row (below) with the to-do's tick in the source mark's place: the title (bodyStrong,
    two lines at most) with its due day or how long ago it came to where it is; line 2, the mark (● needs you breathing
    as on Agents rows, ◐ Working's amber arc, ✓ just finished in the unread green, ◷ waiting) and the hub's line in the
    app's words (a key with arguments, said on the device; a key the app does not know says the section's word, so the
    mark stays); the door to what it asks, "Review ›" or "Review Plan ›" drawn as the style's button (`.themedButton()`,
    one line) until the rows answer in place, a tap on it opening the to-do as the row does; line 3 in caption only when
    it says something besides the project (its steps, the Inactive mark). Never a runtime, engine, model, path,
    repository, PR number or session id. VoiceOver hears two elements: the tick (below), then the rest of the row as one,
    the title, the state its mark draws, the line, the door and how many asks, line 3, and the time in words ("Due
    Saturday", "16 minutes ago"), never "16m".
  - **Pinned headings** (iOS 26, whose plain list gives a pinned header no backing): a section's heading has the page's
    own color behind it across the list's width, so a row scrolling under it never reads as run into its words, as the
    web's pinned headings; iOS 18 draws the system's own strip.
  - **Folds** ("Can wait · 1 ›", "Someday · 1 ›", "Done today · 2 ›") are rows of the list, the whole row their button
    and as tall as a row needs, their words in the titles' column, spoken as `FoldButton` speaks a section's fold.
  - **The tick** is a button of its own: the 24-pt glyph, hit `Space.l` past it on every side (`tapTargetOutset(.all)`)
    without moving the row, spoken as the title with Done or Not done as its value and selected while ticked (HIG-006
    and HIG-007 in To-dos); the rest of the row opens the to-do and is spoken as one element. A tick is drawn
    at once (the glyph filled, the title struck through in the secondary color), with a tap haptic and "Done · Undo"
    for `Theme.Motion.todoUndo`; nothing goes to the hub until that window ends, so Undo, or a second tap on the tick,
    sends nothing. The row then leaves for where the hub files it once the list rests. On a to-do Persona or an Agent
    works on (Working), the tick asks first: "Persona is still working on this. Mark it done and stop?"; on one whose
    Agent waits for the owner's answer (a run's question, plan or tool in its asks), "An Agent is waiting for your
    answer on this. Mark it done and stop it?". Both offer **Done and Stop**, in red since it interrupts work (4.4's
    busy-Agent close), and Cancel, and are presented from the row they ask about. A closed to-do's title stays in the
    primary color, its tick filled; a tick on it reopens it at once. A finished to-do the owner has seen ("Finished
    Tue") ends its line with **Mark Done**, the tick in words.
  - **One trailing swipe**, as the Inbox's (4.2): **Done** in `positive` (the full swipe; it asks first as the tick
    does) and **Someday** in `Theme.somedayTint` (`Theme.Icon.someday`), which files the row under Someday at once, the
    numbers with it, with "Moved to Someday · Undo". A Someday row's second action is **Remove from Someday**; a closed
    row swipes to **Reopen**, a row ticked a moment ago to **Undo**. A row still being saved has none; one nobody knows
    the hub has (below) has **Try Again** and **Remove**. VoiceOver and Switch Control have the swipe's actions and
    Delete… on the row, the menu as its "Show menu".
  - **Long press**: Today, Date… (Change Due Date as the calendar's: `FormDatePicker` with All Day, a timed due kept
    with its time; Remove Date when it has one), Project (the knowledge base's projects, or No Project, drawn at once),
    Someday, then Copy Link (the to-do's web link, `/todos/<id>` on the hub's address), then **Delete…** in red. A row
    ticked a moment ago offers Undo first; a closed one, Reopen. Today keeps a timed due's time of day. Delete… asks
    first, naming the to-do, with **Delete To-Do** and Cancel (4.4, rung 2); the row leaves at once with "Deleted ·
    Undo", and the hub deletes it only once the Undo window ends, since nothing undeletes a to-do there. Message
    Persona… waits for the to-do's own page.
  - **New To-Do** floats at the list's foot (`floatingFooter`), the Theme Color's pill as wide as its words. Its sheet
    (a title, notes with the mic, Add) closes at once: the new to-do is first in To do, its line "Saving…", until the
    hub has it, with "Added · Undo". One the hub turns down is said in a toast whose **Edit** opens the sheet again
    with what was typed.
  - **Writes wait on the device** (`TodoOutbox`): kept on disk with their op id from the tap, sent when their window ends
    (a reopen, Remove from Someday and a new to-do at once; Remove from Someday inside Someday's own window takes it
    back and sends nothing), every one at once when the app leaves the foreground, and again after the hub answers a
    read for one that could not reach it, or that the app was ended with. One sent again whose to-do changed since the
    tap is let go, and said unless the to-do is already as it asked. Until a read of the list begun after the hub's
    answer has landed, the list and every number show it: a deleted to-do is out of the numbers, one put in Someday
    counts there, a new one counts in To do. A change that could not reach the hub says **Not on your hub yet** on
    line 2 in the tertiary color, as "Saving…" is drawn. A new to-do is never sent twice on its own (the hub makes one
    for each request it takes): when nobody knows whether the hub made it, its line says **Not sure it was saved**
    until a read lists it, with Try Again and Remove. A write the hub turns down is dropped and said in a toast; one it
    keeps failing with nobody knowing is said after five tries.
  - **Offline**: while the titles say the hub is out of reach, the list is the one last read, under one caution line,
    "Can't reach your hub · showing what was saved 10:14" (the time the hub last answered a read of it). The line says
    the link itself: the shell's pill stays away while it is in sight (docs/apple/shell.md "Said once").
  - **Undo for VoiceOver and Switch Control**: the toast's Undo is out of their reach (HIG-005), so while either runs a
    held write waits `Theme.Motion.todoUndoAssistive`, and every row's actions offer **Undo “‹title›”** for the latest
    one made on the list while it waits.
  - **Rows never move under a finger**: while the list is scrolled or out of sight a new filing waits, and so does what
    each row says, since a new line can wrap and a door come and go, and a row's new height moves every row under it
    (`Hold`, as the Agents list holds its lines); a row gone meanwhile stays until then. A row that comes to need the
    owner lands at once, alone. When the rest lands, the rows that changed group are drawn in their places at once and
    fade in (`Theme.Motion.conceal`; at once with Reduce Motion), so nothing slides across the list. The numbers change
    at once, without animation.
  - **One number**: the drawer's and sidebar's To-dos row, the folded Pages heading, the bar's Needs you and the Needs you
    header show the same N, from one source; with several assistants it is counted from the current assistant's rows,
    the ones the list shows (`docs/apple/ios.md` "To-dos (V1, #10465)").
  - The page is chosen once, when it is made: a hub that starts listing `todo_view` while today's To-dos is up, or
    while a to-do is pushed over it, changes it the next time To-dos opens, never under the owner's eyes; meanwhile the
    drawer's and sidebar's To-dos number stays today's, so the page and the drawer never say two numbers.
- One grouping control per list (#6662): the web sidebar's Agents block shows the same bar (To-do / Machine / Repo / Project / Status), the same quick filters under it and the same title whether its list is paged or complete, so a pin or an unpin changes nothing above the list; a thing has one name in every list (Machine, never Computer beside it; Project is the workspace, Repo the repository). A grouping that repeats a section above it is not offered (the Agents list's By pinned, which the 常驻 rows draw).
- A paged list drawn by group (web paged Agents list, #6348): choosing a grouping in **Browse by** (Computer, Project, Space; Account for personal agents) draws the list as the sidebar's folding sections, one per group, computers in the hub's order and projects and spaces in a fixed name order, with the group's count on its heading and, while any do, how many of its sessions need the user in the attention tone (the Earlier header's badge). The first three start open on their first six sessions in the list's presentation order (the sidebar's stable computer → workspace → tab → pane order, #11801; newest first elsewhere) and **Load more**, a quiet line in the look of the sidebar's "Show N more" (`ShowMoreRow`, #6662), never a button pill. Activity does not choose row placement. Retained rows and admitted identities are sorted together by current structural placement: full and paged reads admit every new transport identity, while deltas admit identities marked created and previously presented identities re-entering a filter; an older hub that omits `added` keeps legacy admission of every new transport identity. Previous positions and transport timestamps do not participate, real machine, workspace, tab and pane changes apply directly, and identical loaded identity sets reproduce their order after a reload (#11801). A project that is checked out in several places lists them under its heading as quiet lines: its remote, then each directory with its computer and its own count; the same lines sit under the project's name in the value pill's menu, and each of its rows says its computer before its title. A space names its computer beside its own name when the list spans several. The sessions without the attribute are one last section (**No folder**, **No space**). Picking a value in the pill narrows the list to that group and draws it flat. Why sections and not only a filter: a grouping control says how the list is arranged ("List controls" below), so choosing one has to rearrange it; a pill alone left the list as it was until a second choice. Why only a few open: an open section keeps its own list up to date, and a hub can have dozens of projects.
- Side sessions live in their parent's chat (#7944, the owner's request of 2026-11-15, replacing the 11-13 rule that drew them under their parent, always open): a session started from another one that stays a session of its own (a Claude desktop app's side chat, /btw question or spawned session, `parent_agent_id`) is never a row or a count of its own in any list, grouping, filter, Pinned, the dock, Needs you or search results. The hub folds it under its topmost listed parent (`folded_as: "side"`, a side of a side under the same row) when hello lists `sessions_side_fold`, and the parent shows and counts in its folded state: it needs you if a side does, works if one works, and is unread if one is unread while it does not work itself (sides only; a Codex child or a worker does not make it unread). The parent's chat reaches its sides from one control ("Side sessions panel", 3.3). Why not keep them in the list: they made a list of conversations the user started into a list of every question asked on the side, and the parent's state already says when one of them needs the user. Without the feature (an older hub) the web keeps the 11-13 arrangement: drawn right under their parent, a level in (`SESSION_LIST_STYLE.nestedRow`), with their own rows and counts.
- Older rows folded by activity (web paged Agents list, #4236): an "Active within" menu beside the list's count (Any time, the default, then 24 hours, 7 days, 30 days) folds the rows last active before the window into one Earlier group at the list's end, folded by default, with how many it holds and how many of them need the user on its header. It is a display choice ("List controls": grouping and display), not a filter: no row leaves the list, no count changes, and Any time undoes it. Why not a filter chip: a filter hides rows, and sessions that need the user must stay visible (below).

#### Long lists on a detail page

A section of a page about one thing (a computer's page) lists its items in place only while there are **three or fewer**. From four, the section folds to one summary row: how many, then the one or two facts that need attention, with a disclosure chevron (#5821, Browser Sign-ins on iOS `BrowserLoginsSection` and the web `BrowserLogins`: "48 sites" over "First expires 11/24/2026 · 2 expired", the expired count in the caution color). The section's own header keeps its title, its count and its main action (Import); nothing else of the list stays on the page.

- **Where the list opens**: iOS pushes a page of its own (`BrowserLoginsListView`, a `List`); the web opens it in place under the summary row (`Collapse`, the list held to 60% of the viewport and scrolling), the machine page's way of showing more (Node health's "Show all checks").
- **The whole list**: a search field that filters by name, applied 150 ms after the last key; an order, by name by default, with the one that matters for the list as the other choice (Expiring first: expired on top, those without a date last); a mark on the items that need attention (an Expired chip in caution, as the brief box's Overdue in 3.3; not red: nothing failed).
- **Row actions**: a swipe and the long press on iOS, a row-end ⋯ menu on the web (and on iOS's in-place rows), never a button on every row. Several at once: Select (iOS edit mode, Select All / Deselect All and "Clear N" in the bottom bar) or checkboxes with a select-all box (web). Every destructive action asks first (4.4, rung 2), naming the item, or how many.
- Why three: the session's file card shows three rows before "Show N more files" (3.3), and on a phone a fourth row with its date already pushes the sections below a screen away. The chat's queued messages show four in full because each is one line without actions.

#### Sessions that need the user

In every grouping a list offers, a session that needs the user is visible without opening anything: in a section at the top, or by the state mark on its row, with a folded group counting it on its header. Grouping by location (computer, repository, worktree, tab) never hides it, and nobody has to pick a computer or a repository first to see their own work.

The web's To-dos page (#11838) files to-dos by their state, so one the sidebar's To-dos badge counts can sit outside Needs you: a to-do no Agent is on, under Other open. Each row the badge counts carries the badge's mark, **Up to you** (`Tag` in the accent tone the badge is drawn in), why it counts in its tooltip and spoken label, and a folded heading over one says how many ("1 up to you"). The marks and the badge come from one hook (`useOwnerNeeds`), so they never disagree. Why a mark and not a section: what the badge counts spreads over several sections (Needs you, Parked, Other open, Just finished, Working for work held for the user's OK), and Other open also holds to-dos it leaves out (a goal its pieces carry). Why not Needs you as the mark's words: that is a section of its own with a narrower rule (principle 4).

A child session (an Agent another Agent started, folded under a listed parent; web, #5120, after cindy) is never a row or a count of its own: its parent's row carries it, in the parent's folded state (a waiting child makes it need you, a working one makes it work), its second line saying what the child waits for. The child's need is visible on that row, counted once under the parent.

A side session folded into its parent (#7944, `parent_agent_id`, with `sessions_side_fold`) is the same: its parent's row carries it in the folded state, and the parent's chat header carries a needs-you dot on its Side sessions control while one waits, so a waiting side is visible without opening anything. An older hub that does not fold sides leaves each a row with its own state mark, counted on its own.

A herdr tab's workers are the same (web, #5180; the owner asked on 2026-11-12 that this follow cindy's Team exactly). In a tab with several Agents the lead is the Agent in its first pane and the tab's other running Agents are its workers, folded under it by the hub. A worker is never a row, dock entry or count of its own; the lead's row carries it as above. The lead's chat shows no strip of pane tabs any more: its workers are in the **Team** side panel (section 3.3). A tab whose first pane is a shell has no lead, and its Agents stay rows of their own.

#### Finished work nobody has seen

With Session Triage on, a turn the triage found done leaves Unread, but while it is new it is not filed with the
seen ones (#9751, `docs/protocol/agent-states.md` ("Just finished")): for three hours, or until it is seen, it has a
By Status section of its own after the work still going (Working, Waiting in the background) and before Waiting,
Muted and Seen, and it keeps its space, repository and worktree out of the idle folds. Its mark is the unread green as
a plain dot, its state word "Just finished" (the To-dos' section word, `StateName`), its second line the triage's
"✓ conclusion". A muted one is plain Seen. Why not Unread: Unread stays what needs a look. Why not plain Seen: work
that ended minutes ago sank under sessions seen days ago. Why after the work still going: as in the To-dos' order
above, what still runs comes before what is done. The web and Android (#9786; there the dot is Unread's, as Awaiting
reply's) have it; Apple and HarmonyOS follow in their own issues and show these rows as Seen until then.

#### The work-item row

The Agents list, the Inbox's Agent section, the Assistant home and history show a session with the same content, in every Agents row style (Avatar, Two Lines, State Cards, Compact List; iOS's ⋯ › Display and Settings › Appearance › Agents List Layout, Android's ⋯ › Display and Settings › Interface style on Glass, where the parts each style draws come from `rowParts` in core/domain, #4338 A9):

- Leading: one source mark, the Agent's brand or a contact's avatar. The row's one state sits on its corner, chosen by the order in section 5.6. iOS (`AgentAvatar`) and Android's Glass row put the mark in a round well with the state's dot on its corner, rimmed apart from the well.
- Line 1: the session's title, with the time on the right, localized ("6 min ago").
- Line 2: when the session needs the user, what it waits for (the plan's title, the command, or the question itself); otherwise its latest line, in the secondary color. For a failed run, what failed, in red.
- Line 3, optional: computer · project, leaving out what the grouping already shows, then the model's short name. iOS's Avatar row and Android's Glass row (#4338 A9) draw it as the source tag, the tab (space › tab outside the tree), the computer only while the list mixes computers, the model, the to-do as an outlined chip and the muted mark; Android's words come from `rowContent` in core/domain, its model's short name from `modelShortName` (the web's `modelChipLabel`; iOS's row still shows the raw string). On the web, an associated terminal tab is secondary metadata too: a localized **Tab {name}** chip in the complete Agents list, and a visible part of the sidebar row's secondary text even when age, source or to-do marks would hide an ordinary hint. When the tab is primary, it takes the title line without a duplicate chip. The localized **Tab {name}** prefix reads `slot_name` only beside a `slot_id` / `tab_id`; without that association it can be a workspace name and never gains the prefix. Existing structural tab fallback text is unchanged. Search reads the same `slot_name`, so a rename changes both recognition and the words that find the row.
- Never in a row: raw model IDs, session IDs or engine badges. At most one count badge.
- Answers in the row: a question's choices, and a tool approval whose full command is shown and whose action is low-risk (Deny, Allow Once). Plans, high-risk actions and Always Allow for This Session open the session, where the context is.
- Views grouped by location draw these rows too (#3753): By Repository's computer, repository and worktree are lines of their own, each worktree's sessions in these rows under its line. The Mac's By Repository cards (frozen) keep their rows (`WorkspaceAgentRow`). On iOS and on Android's Glass style each repository is a card: its header has the folder, the bold name and the chevron, no counts, with its actions in its long press (`RepoCardHeader`, Android `AgentRepoHeader`); a worktree's line has its mark, its name, a Primary chip on the main checkout, the branch when the name is not it or Detached HEAD as a caution, "N agents" or Idle at the end, dimmed while its computer is offline (`RepoWorktreeLine`, Android `AgentWorktreeLine`, #4338 A7). A computer's repositories with nothing going on follow its busy ones under Idle Repositories with their count, one card with a row each: the folder, the name, "N agents · tab names", when it last moved and a chevron, a tap opening its agents under it (`QuietSpaceRow`, Android `AgentIdleRepoRow`). A busy worktree's idle agents fold behind one line, "2 idle · review · research", and once opened "Hide N Idle" after them folds them again (`RepoIdleLine`, Android `AgentRepoIdleLine`, #4338 A8). By Project's spaces fold theirs behind one "N more idle" row the same way: it opens them and, once open, "Hide N Idle" follows them and folds them again (`collapseRow`, Android `AgentIdleRow`'s `open`, Glass).

#### List controls

A list's controls come in four kinds. Each kind looks like itself, so that narrowing a list never looks like leaving it.

| Control | What it decides | Rule |
| --- | --- | --- |
| Scope | which computers or hub | a menu that names what is shown |
| Filter | which states, sources or dimensions show | each active filter shows as a chip that removes it; Reset clears them all |
| Grouping and display | how rows are grouped, sorted and drawn | a grouping control or the ⋯ menu, never a filter chip |
| Navigation entry, such as personal Agents | opens another page | a row or the sidebar, never chip-shaped |

Search may take query operators and scopes; they live inside the search field.

A list keeps no standing search field of its own beside the app's one search (#6589, the web's Agents list): **Search chats** (⌘K) finds its rows with the list's operators (`machine:`, `project:`, `harness:`, `model:`, `state:`, `is:pinned`, `since:`), a match on the row's name first, then its project or computer, then what is inside it, and its **Filter Agents list** (⇧↵) puts the query on the list in place, as a filter chip whose text reopens ⌘K. ⌘K's own **Filter** button, at the end of its field, offers the list's filter choices (computer, project, status, harness, model, Pinned, updated within) and writes each into the query as its operator, the number of operators on it, so a choice stays text to read and change and no hint line has to name the operators. The list's filter button sits on the list's title row, with the list's other menu buttons, and says how many filters are on, as iOS keeps its Agents filter (the computer) in the top bar's list menu with the chosen one a chip over the list.

- **iOS Agents filter capsule** (#7033, owner-approved final E; whole labels, #9121): `CapsuleFilterBar` below search is one fully rounded surface (`Theme.card`, `Theme.cardStroke`, `Theme.Size.hairline`). Android's `JAgentFilterCapsule` (`AgentFilterCapsule.kt`, #7034) follows the same rule. Only the existing quick filters scroll horizontally; their availability, counts and defaults are unchanged. **Every filter shows its whole name, always**, in every language and at every text size: never two characters and an ellipsis, never a name cut to fit, and a chip is as wide chosen as not (no focus or width animation). A strip whose names do not fit scrolls; the web's quick filters show whole names too, wrapping onto more lines. An edge with more past it fades out over `Theme.Size.fadeEdge` (`.edgeFade()`, a mask, as the space tab strip's): the trailing edge while content lies past it, the leading edge once the strip has scrolled from its start, so a cut-off chip reads as "more"; where nothing is hidden the chips are drawn as they are. The chosen filter keeps its highlight (`Theme.FilterCapsule.selection`, `Theme.Motion.standard`) and scrolls to the strip's middle where it can; counts never truncate. Reduce Motion turns the highlight's animation and the scroll to a chosen filter into plain changes, and the fade has no animation of its own, so it snaps. At the trailing end, a 44 pt `Theme.Icon.capsuleGrouping` control stays fixed behind a hairline of `Theme.FilterCapsule.separatorHeight` (22 pt); its menu contains only `DevGrouping.offeredNow` and checks the current grouping. The former separate grouping pill is removed. Personal Agents keeps its separate navigation entry, outside the capsule. Spoken filter labels remain complete, including counts and selection. Why: the owner requested one compact row without mixing grouping choices into state filters; this serves people monitoring Agents, not Assistant. #9121 dropped the two-character label of #7033 and #7034: in Chinese and Japanese it saved no width and could change the meaning (完成未看, "done, not seen yet", read 完成…, "done"). The Mac's segmented control and the web are unchanged.
- **Inbox section bar** (#9053, `docs/assistant/inbox.md` "Section bar"): a filter of states, drawn with each client's quick-filter component (iOS `CapsuleFilterBar`, Android `QuickFilterBar`, the web Agents filter bar's look) with Needs you, In progress, FYI, Done and All. Needs you and All always show; the others while they have rows or are chosen; Collapsed stays under All. Needs you carries the `.needs` `CountBadge`, In progress and FYI the neutral one, Done and All none. It opens on Needs you on each entry and is not kept across visits; All is the stacked page as before, the filter's reset. Grouping sits in the bar's trailing menu, not in a row of its own; on Android it is "Group by" in the Inbox's ⋯ sheet, since `QuickFilterBar` has no trailing control and its five choices already run past a phone's width (hidden while In progress or Done is chosen, as the menu is). Why: the stacked page put FYI and Done several screens below a long Needs you; a bar that lands on Needs you keeps what needs the user first and one tap from everything else (3.4 "Sessions that need the user": the Needs you count stays in sight from every choice).

- **HarmonyOS Agents groups** (#8966): the existing native grouping Select offers By Status (default), By Machine and By Project. It reuses the native list, Select and 48 vp control tokens, not a new style or a second flat/repository/worktree taxonomy. The shared Rust projection owns pinned, quiet, folded and filtered rows and their counts. Group headers expose the current localized state/machine/space title, state counts and a 48 vp Expand/Collapse control; quiet projects have an explicit idle-projects heading. Default idle sections stay folded; the header expands them and their rows with the core's `folded`, `expanded_idle` and `expanded_quiet` settings. A folded header still exposes Needs You. All labels ship in en, zh-Hans and ja. The previous native sorter and debug shadow are deleted; no release-only platform-specific sorter remains. These rules serve people monitoring Agents and follow iOS, rather than adding an independent HarmonyOS presentation.

A list row found by what is inside it, not by its own name (a Persona chat found by a message, #6283), keeps its title and puts the sentence that matched in place of its second line, the match marked as search in chat marks it (the accent tint), the line starting a few characters before the match so a narrow row still shows it (each client cuts for its own width, #7045); opening it goes to that message, ringed as a search hit. A row found by its name keeps its usual second line and opens as usual. Local matches show at once; what the server finds merges in, and a short line under the field says when the server's results may be incomplete.

- Section headers (`JSectionHeader`) measure the complete title, count and ordinary actions in one layout pass. Only when they do not fit, the actions move into a `JMenu` and the title wraps as needed so the title and count stay fully visible; fitting layouts stay pixel-identical. The overflow decision is made during measurement, never by switching after a truncated frame. Button labels still never wrap.
- Command rows in the composer's "/" list (`ComposerSuggestions`) measure the complete command and detail. Only when they do not fit, the detail moves below the command and wraps without a line limit; fitting rows stay pixel-identical. Quick-message and @ rows keep their single-line layout (#7322).

Android's `JMenu` takes a choice list in groups: an action can open a group with a title (`JAction.section`, drawn after a rule; `JActionSheet` draws it too, over its labels' column, as the Agents ⋯'s Display) and carry a quiet line under its label (`JAction.subtitle`), as the To-dos list's Show (each choice with how many it would show) and Sort by (#3740). A status section's count can sit in a capsule of its state's tone (`JSectionHeader` `countTone`, the To-dos list's sections, #3712), as iOS `CountBadge`. A one-line notice over a list keeps its action on its line as a text button (`JNotice` `inlineAction`: "1 more waiting on you, not in This week · Show all"). Above 1× text, `JMenu(multilineActions = true)` wraps a menu option and its subtitle instead of ellipsizing the count, destination, verb or scope; the pull-down stays anchored at every text size and the default one-line rendering is unchanged. First use: the Inbox's FYI / Collapsed group actions (#10425), because an anchored `JMenu` caps their width.

### 3.5 Navigation

- Shell (compact width): Assistant and Agents as pages on a strip, a drawer sidebar, no tab bar. Top bar: persona button, `ShellSwitch`, circle button (`apple/Persona/DesignSystem/ShellComponents.swift:19,258,294`). Re-tapping Assistant opens its menu. Explicitly selecting Agents opens its list, clearing any retained detail navigation; re-tapping Agents from a detail returns to the list. Re-tapping Agents while already at the list opens the existing list-options menu. A card, notification or session link remains a targeted open and goes directly to that Agent, not through the list. This list-first rule serves people moving between their everyday Assistant conversation and Agent work, without a setting or a new visual style.
- Sidebar places and tasks (#6599, one-button amendment #7799, `docs/design/sidebar-navigation.md`): a place chosen in the drawer (a conversation, Inbox, To-dos, Scheduled Tasks, Knowledge, Machines, Recordings, extension pages, Profile) switches the page under the open drawer without animation, then the drawer closes; each place keeps its stack while the user is elsewhere, and choosing the place in front returns it to its root. On iPhone a place's root shows ☰ (`ShellPlaceRoot`) in place of Back; its sub-pages show Back and no ☰. Return to the conversation by choosing one in the drawer, or with the root's system edge-swipe (Back is hidden but the existing pop-gesture delegate remains enabled). A sub-page's edge-swipe goes back to its root. The drawer's sideways pan still yields every pushed page to that system gesture, so the two never compete. A place has at most one top-trailing action on iPhone: Inbox's Select, Knowledge's filter and map, an Inbox item's Done, a Lark chat's Pins and summary, and Browser Sign-ins' Select join their existing ⋯ menus. The multi-hub Inbox's picker joins that menu, and only on iPhone does selection's Select All join the bottom action bar; outside the iPhone shell it remains at the original top leading, verified by `IPadInboxSelectionTests`. These consolidations do not change iPad or Mac layouts. A task (Settings, Archived, Auto conversations, Search) opens over the open drawer and returns to it; a conversation picked in one opens after its sheet has gone. The drawer's closing never runs with a push, a slide or a sheet.
- A page's top-leading button follows how the page was reached, not what it is (#8603, `docs/design/sidebar-navigation.md` section 6): a page picked in a sidebar or list is a root and shows the button that opens the sidebar (the main conversation's `Theme.Icon.sidebar`, "Show Sidebar"; the persona's face in a phone's drawer; under side bars the side column's first item, where a sub-page has Back, #11636), never a ‹; a page opened from inside another shows ‹, which goes back to it. The same Agent's chat is a root picked in the sidebar and a drill-down opened from a waiting Agent on the main conversation or in Inbox, a task's card, a Brief card, a to-do, a knowledge project or a meeting (#9971, #12006): it never moves the sidebar's selection.
- **A presented page leaves at its top-leading corner** (#9653, the owner 11-17): a sheet, a full-screen cover or a self-drawn layer over a page puts its way out at the top leading edge, an ✕ (`SheetClose`, 3.6); a pushed page keeps its ‹ there. The trailing edge holds only the page's own actions: its ⋯ menu and its confirming action. An edit sheet's ✕ leaves without saving and its ✓ saves, as iOS 26's Calendar and Reminders lay it out; Save, a Done that saves and Add become that ✓, and a verb that says more (Send, Connect, Install, Commit) stays a word in the same place. A page that only shows things has no Done: its ✕ closes it. Not covered: a question sheet whose Cancel sits under its answer in the content (Close and Delete Agent, consent sheets), the system's own sheets (share, camera, document scanner), and a medium-detent peek with a drag indicator and no bar. Web and desktop dialogs keep their ✕ at the top trailing corner, the convention there; Android follows in #9661.
- Regular width: system tab view and split navigation; the same sidebar in a column. On iOS in the Separate layout (#4206) each tab shows its part of the one sidebar's column (Persona's: the main conversation, the pages, its chats; the Agents tab's: the Agents list), with the same head (its fold at the trailing end), foot, width and color, and the system's tab bar on the iPad; on the open iPhone Duo the upright switch stands in its place (below, #4231). On iOS (#4206) the column has one width (400 pt on the iPad, 320 pt on the open Duo) and lies beside the page where the page keeps at least `Theme.Size.splitPageMin` (400 pt, a large phone's screen), over it elsewhere (the open Duo and an iPad mini, upright); turned to over, a column that was out goes away, and turned back it comes back as it was left beside the page. On iOS, selecting or reselecting Agents follows the same list-first rule, leaving the split's detail empty until an Agent is explicitly opened. Agent sidebar links use the bound detail path, not a separate implicit split selection, so clearing navigation also clears a row-opened detail without rebuilding the list. Cold-launch and width-change restoration remain separate from explicit tab selection.
- Assistant switcher on Apple (sidebar header, `apple/Persona/Views/Assistant/AssistantSwitcher.swift`, once a roster is known (read in this run, or kept from the last launch) and the current assistant can be shown, including with one installed assistant; the rules every client shares are in the "Assistant switcher" bullet below): the persona face with a state dot at its foot, the current assistant's name with a down arrow and its state line; a tap opens a popover list of the hub's assistants on a solid panel (`Theme.card`: through glass the badges behind it showed beside the rows), each row the first letter on a solid disc in its color with a state dot at its foot (green online, gray offline, orange installing, ringed in the panel's color), name, Default chip (only beside another assistant), state line (up to three lines at the accessibility sizes) and a check on the current one or, on any other with something waiting, its count (`CountBadge`'s `.needs` look; VoiceOver says "n things waiting for you"); an offline row is dimmed as a whole (`Theme.Fade.offline`) and still pickable. While another assistant has something waiting the face carries a mark at its head (`AssistantWaitingDot`, the needs-you color, ringed like the state dot; beside the name where the face is not drawn), and the header's spoken name says so. A page of an offline assistant's merged things (the inbox, to-dos) shows `AssistantOfflineNotice` in place of its empty state: laid out as `EmptyStateView`, with the assistant's avatar and its offline mark (`AssistantMonogram`, faded as an offline row) where that has its symbol, then "<name> Is Offline" and when it last synced. Then New Assistant and Manage Assistants…, which stay whole and tappable at the largest text size while the rows scroll above them. Rows are at least 44 pt and one line each. Why a popover and not a `Menu`: a row carries a colored avatar, a status dot, a chip and a second line, which a system menu item cannot draw, and the same list is drawn on web and Android. The sidebar and Settings reuse this same popover/list; pages do not add another switcher implementation.
- Android regular windows keep that shared sidebar beside the Assistant stack for Inbox, To-dos, Computers, Knowledge and extension pages. One `AssistantPaneFrame` outside Persona's `NavDisplay` owns the sidebar. Picking a place replaces that page's root with no push (#9667, `docs/design/sidebar-navigation.md`); a drill-down still pushes, and Back from it returns to its root. System Back at a place root returns to the conversation as a root switch with no animation; system Back at the conversation leaves the app. A page handler on that root (Inbox selection, search, a sheet) runs before that place-root Back. The conversation and the fixed places (Inbox, To-dos, Knowledge and Computers) stay composed while another is in front, so the conversation keeps its non-saveable history, local operations, running effects and list anchor, and those places keep their list, filter and scroll. Folding inserts the conversation under that same list and unfolding removes it, so that page is not rebuilt. A to-do opened as its own root, or an extension page, keeps only its path and is composed only while it is in front. Folding that root copies it onto the conversation and leaves the fixed places composed. A place the phone drawer pushed over the conversation is promoted onto its own host when the window becomes regular, and that promotion rebuilds the page. Wide phones retain their existing Inbox list-detail without a persistent sidebar; regular windows never create a third Inbox column.
- Page titles: `.pageTitle`, `.detailTitle` (`apple/Persona/DesignSystem/PageTitle.swift`).
- An Agent chat's header folds into a slim bar (iOS `ChromeHeader` and `compactChrome` in `apple/Persona/Views/Chat/ChatView.swift`, `docs/apple/ios.md`; Android on Glass `JFoldingHeader` and `ChatSlimBar`, #4339 C4): it follows the list's own scrolling (toward the newest it folds, back into the history it opens), is never left half way when the finger lifts (folded at the newest end, else the nearer end), and folds by itself after 3 s without a touch while nothing is presented over the page. The slim bar is back, a centered capsule (the agent's mark, the state's dot, the title; Connecting… while the hub is out of reach) that opens the header again, and the open header's ⋯ on its right (#7962): the same menu from one definition, drawn bare like the slim bar's back (no glass: the bar is `Size.slimBar` tall) in the open ⋯'s `Size.tapTarget` square, as wide as back so the capsule stays centered. The open header's second line leads with the state's words (iOS `ChatView.stateWords`): "Working N min" while a turn runs ("N hr M min" past an hour; the work line has the seconds), the state's name otherwise, "Working (Background)" once the turn ended and a workflow keeps the agent going, "Waiting in the background" when all it runs only waits; that wait is drawn with the clock for its dot, in the lighter tone, in the slim bar's dot too. Android also opens it when a new request arrives, and never folds by itself while animations are off. The open header's title opens Session Info.
- Sidebar layout (Settings › Appearance › Sidebar: Merged, #6332; the drawer on a phone, a column at regular width; on a
  phone a page it opens has the persona's face where a back would be; with nothing picked Merged at regular width and
  Separate at compact width, #4206): the web sidebar's structure in one column
  (`UnifiedSidebarColumn`): the assistant, the main conversation held at the top, then one list (Pages and
  Persona Chats folding under their headings, then the Agents with their heading's ⋯ / Personal Agents / New Session, quick
  filters, grouping, Needs You and groups, the Agents page's own rows; the three headings are one row each in one box,
  `SidebarHeadingBox`: a sidebar row's height and padding for the name, `Space.s` over it, buttons beside the name never
  setting the height, and no sections' gap before Agents, so the three stand evenly apart folded and unfolded; #10551,
  `SidebarHeadingGapsUITests`), and at the foot the Needs You jump, the usage, New
  Chat (the only one where the column is beside the page, #4206), and Search and Settings in one capsule. A pick replaces
  the page beside it. **Search** is one page over the whole window (#4206, the owner; it replaced a field in the head
  that filtered the column in place): the field at its foot, two scopes at its top, Chats (Persona's, by what was said
  too, #7123) and Agents, starting on the side that is in front; no "All" scope yet. Who it serves: anyone looking for a
  conversation or an Agent, who gets one place to look and sees which kind they are looking through. Two pieces are new: **the Needs You jump**
  (`SidebarNeedsJump`): "N need you" with an arrow toward the section, a capsule in a band of its own above the column's
  foot that never covers a row and stays while anything waits (no foot jumping as the list scrolls); it is a way to the
  one home of those requests, not a second home, and counts what the switch and tabs count (`agentsWaiting`). **The usage
  strip** (`SidebarUsageStrip`): the two fullest subscription accounts as name, percent and `UsageMeter`, the web
  sidebar's foot (#2432); a tap opens Machines. Separate stays the default and pixel-identical.
- Android (#6378) offers the same **Separate** and **Merged** values under **Settings › Appearance › Sidebar**.
  Separate is the Android default at every width. Merged uses the current left drawer and the regular window's
  existing beside or overlay policy. Main conversation stays above the scrollable Pages, Persona Chats and Agents.
  The first two sections keep their folds on the device. `JSidebarSectionHeading` gives all three headings one
  sidebar row's height and `Space.s` above. The Agents list supplies its rows, filters, grouping and actions, with one
  selected row for the visible root. The fixed Needs You jump uses the shell count. The two fullest subscription
  accounts share their data with Computers. One New chat control and the Search and Settings capsule sit below them.
  `JSidebarFooterActions` stacks the capsule under New chat when the full label needs the row's width.
  Search opens over the whole window in Chats or Agents. The Android guide lists the pages its capabilities expose.
- One selected row in the Sidebar layout's column (#11032; `SidebarRootsUITests.testOneRowIsSelectedAndItFollowsTheRoot`):
  the whole column (Main Chat, the Pages rows, Persona Chats, the Agents list with Pinned) has at most one selected row, and
  it is the root of the page beside it (`UnifiedSidebar`: `atRoot`, `shownPlace`, `shownAgent`, all read from its roots). A
  pick in the column moves it; a page opened from inside the page (a link, a card; pushed, with Back) does not. Every row
  draws the same shade, `SidebarSelectedShade` (`apple/Persona/DesignSystem/SidebarSelectedShade.swift`: `Theme.Fill.well`
  on iOS, `Theme.Fill.macSelected` on the Mac, in `Theme.Sidebar.radius`), and says `isSelected`; in the column an Agent's
  row is shaded in every grouping, By Repository's included (its blue tree wash stays outside the Sidebar layout).
- Side rail for iPhone Duo (`apple/Persona/DesignSystem/ShellSideRail.swift:50`). Under its side bars the shell's pages never move sideways: they cross-fade instead of sliding, and the drawer's sidebar slides in over the page, which stays (`docs/apple/ios.md`, "iPhone Duo").
- Tab bar for iPhone Duo (#4231, #10362; `ShellSwitch(axis: .vertical)`, `apple/Persona/DesignSystem/TabSwitchDock.swift`): the open Duo (regular width) shows the system's tab bar on the tabs, placed by iOS (along the foot upright, down the side column on its side), and draws none of its own (#10362); what follows describes the outer display's rail, and its parts about the open Duo are history. In the Separate layout the Duo had one tab bar on both displays, after a tab bar down the side of the outer display (the owner's reference, 2026-11-17). Its items are the page's symbol over its name (Assistant, Agents, Messages when it is on; 56 pt, more than a tap target), the chosen one's symbol white on the Theme Color's round fill and its name in that color, the others a line symbol over a plain name, in one glass capsule (`.surface(interactive: true, in: .capsule)`) with a hairline edge (`Theme.cardStroke`) and a faint shadow (`StyleSpec.softShadow`). It is a bar of fixed items (6.2): the default text size at every size, each item shown large on a long press, the capsule one container VoiceOver reads as a tab bar. Down the side (the outer display's rail; the open Duo on its side, in the system's tab bar's place, which is hidden on a phone) it stands in the system's column, centred on the status bar (`Theme.Shell.sideColumnEdge`) and in its lower middle, not at the foot: on the outer display's rail above the face and the page's action, which stay at the foot within the thumb's reach, on the open Duo above their room, left empty (`ShellRailButtonRoom`), so it stands at the same place on both displays and stays put as the Duo folds or unfolds. One set of values on both: the column's foot to the lowest button `Theme.Shell.sideRailFoot` (16 pt), the buttons `sideRailSpacing` (12 pt) apart, the bar `sideTabBarGap` (8 pt) more above them, so its foot is 144 pt above the safe area's (178.7 pt above the screen's foot on both displays at the default text size), its edge 17 pt from the screen's (centred 49 pt in, as the clock) and 3 pt inside the column; the open Duo's rail takes the shell's text size as the pages' rails do (`shellTopBarScaled`), so the items' names are drawn alike at every size (`DuoTabBarAlignmentTests`). The two displays differ in height (678 / 669 pt) and in where their status bar is (the outer display's starts 52 pt lower), so the bar's distance from the top and from the status bar is not the same on both: it is anchored to the foot, where iOS anchors a tab bar and the thumb is. with the bars across the top (the open Duo upright) it runs along the window's foot, centred, and the pages keep its room as for the system's tab bar. Over the tabs it stays put on a push and a pop. While the keyboard is up it fades away. A newly chosen item is reached by a lens (#9837, `TabBarLens`, `lensSurface`): it leaves the item that was chosen, swells and stretches along the bar, glides there with a little bounce (`Theme.Motion.lens…`) and fades as it lands, clear glass over the items in the glass styles (it bends them as it passes) and the soft pill under them elsewhere; at rest there is none. With Reduce Motion the items cross-fade (`Theme.Motion.tabFade`). The chosen Assistant item shows the persona's face (still), as the reference's chosen home item shows its brand. A page's count is white on red on the rim of its symbol's circle, at the top trailing corner (3.8): Agents' Needs You (`agentsWaiting`) and Persona's (`TabBadges.assistant`), the numbers the system's tabs showed; Messages has none, as its tab had none. Why one component: the inner and outer displays look and behave the same (the owner, #4231). The iPad keeps the system's tab bar; an ordinary iPhone keeps the top switch.
- The side column on iPhone Duo is laid out as one (#9837): the system puts a page's own bar items down it from its top (Back alone, a 48 pt glass, then the rest in one glass, 48 pt items 4 pt apart, from 174 pt under the safe area's top on the outer display and 124 pt on the inner one, `Theme.Shell.systemBar…`) and a page's search at its foot; the tab bar and the rail (the face, the page's action) stand from the foot. Nothing in the column lies over anything else (`DuoSideColumnTests`). Where a page's buttons would come within `Theme.Space.s` of the tab bar, its trailing buttons are only in its ⋯ (which has them all) and the leading ones that fit stay, Back and ⋯ always (`TabSwitchLayout.pageButtons`, the room from the tab bar's top as the rail reports it, `SideColumn`); with room all of them stay. The tab bar is never moved or made smaller for a page's buttons. A page with a search keeps the rail's foot slot for the system's (`ShellRailSlot`), and its other actions go with its buttons at the top (Messages' Lark Settings).
- Platforms: web phone shell matches (drawer plus top-bar switch); Android `HomeNavigation` has a 36 dp segment height versus Apple's 44 (DS-18).
- Web list and detail in one column (the Inbox on a narrow page, web usability audit item 74; `docs/web/web.md` "Pages", **Inbox**): where an opened item takes its list's place, the keyboard goes with it. Opening a row by keyboard puts the focus on the item's title, a heading with `tabIndex=-1` that shows the accent focus ring (the item's pane while it loads), so a screen reader names the item. **Esc** goes back to the list as ‹ does, except over a dialog or a menu, inside a text field, and where the list is beside the item. Back on the list the focus returns to the row the item was opened from; when that row has moved (the item marked done from its own header), it goes to the row now in its place, and the list does not scroll after the item. A wide page, with the list beside the item, leaves the focus where it is both ways. Why a rule: the list's row is hidden with its list, so without it the focus fell to the page's body each way; another list-and-detail page that folds to one column (the To-dos) follows the same rule.
- Android home selection (#5638): choosing **Agents from Assistant** opens its list root, even when a session, request or terminal was open there before. Keep the existing root and list state, and keep Assistant's current conversation. A task/session entry or notification/session link still opens its requested detail directly; Back from an Agent opened in Assistant returns to the originating page. Re-tapping an already selected Glass segment still opens its menu. This separates a primary-destination choice from targeted detail navigation without a setting or new component.
- Web assistant identity appears in navigation whenever the hub supplies a displayable current assistant, including with one installed assistant. The desktop sidebar and phone drawer head show the current assistant's name, first-character avatar with its status dot, and status/computer line, and open the roster menu. Web remembers the last header id and name per hub, including the default assistant's, for display only: a reload or transient roster-read failure keeps that identity instead of flashing the product header, while a genuinely unsupported roster route (404) clears it and returns to the product header. An empty roster or no displayable current identity also restores AppMark, Persona, connection/computer status and the original Appearance/other-hubs/Settings/Disconnect menu. `ASSISTANT_MENU_WIDTH` in `web/src/ui/menu-model.ts` is 264 px for the assistant menu and 220 px for the product menu. Android keeps the assistant's name, avatar, status and sidebar switcher with one installed assistant, without a Default tag. With one installed assistant and a roster, Settings' first group keeps "<name>'s settings" and **Switch**, and Settings › Assistants still provides New assistant. Web conversation titles and message identity still follow `multiManager`: with one assistant or no roster they keep their existing product/session titles and messages add no roster identity. Showing this identity does not enable filtering, badges, extra requests, manager headers/query parameters or `needs_you` reads; those remain under `multiManager`. The user's chosen Persona look stays unchanged.
- Assistant switcher (Android: one or more installed assistants; web: a displayable current assistant identity, fresh or cached): the head of the sidebar is the current assistant (avatar, name, status line, a chevron) and opens the roster: installed assistants as a single-choice list (the default tagged only when more than one assistant is installed, the current one checked, a count on the others, an offline one's whole row faded to 55% and still selectable), the ones still installing, then New assistant and Manage assistants… (`docs/assistant/managers.md`, "The current assistant"). Web: `web/src/views/AssistantSwitcher.tsx`, a `Menu` whose rows take a lead mark, a tag, a count (with the words a screen reader says for it, `badgeLabel`) and a faded state (`dim`); the row's earlier entries (appearance's three rows under a heading, other hubs, Settings, Disconnect) follow under a divider. With one installed assistant, web shows the same head and a one-row roster with no Default tag; New assistant and Manage assistants… remain in the menu, so creation is not confined to Settings. The menu scrolls, and no submenu opens over its last rows, so every row stays reachable at 200% zoom and the largest text size. Settings' first group ("<name>'s settings") titles itself with the same avatar and name, and its **Switch**, a text button at the title's end, opens the same rows (web: `AssistantSwitchButton`, `views/settings/SettingsGroups.tsx`); the rows of that group read and write the current assistant only. Why a new pattern: the assistant is a scope for the whole app, not a page, so it sits above the pages rather than in the assistant page's top bar where the segmented switcher was. Android: `AssistantSwitcher` opens the roster in `JMenu` (its content slot holds the rows above New assistant and Manage assistants…), as wide as the sidebar and over its rows, closed by a tap outside, scrolled when it does not fit.
- Apple keeps the persona name, circular avatar, sidebar identity/status footer, personalized composer prompt and voice attribution with one assistant or a legacy roster. Once a roster is known (read in this run, or kept from the last launch) and the current assistant can be shown, the switcher appears with one installed assistant too: a one-assistant roster has no Default chip, and New Assistant and Manage Assistants… remain below it. Without a roster, the sidebar keeps the persona's name as its title. Settings independently titles its first group **<name>'s Settings** and shows **Switch** whenever the hub answers a roster, including with one installed assistant; Switch reuses the sidebar's popover/list rather than implementing a second switcher, and the single-assistant list keeps its Default chip hidden. A legacy hub without a roster keeps the neutral **Current Assistant's Settings** heading and no Switch. #4689 withdrew the Apple identity-hiding change in #3975 at the user's request; #5616 shows the switcher with one assistant on every client.

- Android Settings (#5233) keeps the current-assistant / this-device / Hub scopes and reuses `JSearchField`, `JListPage`, `JGroupItem`, `JRow`, `JSectionHeader` and the assistant switcher. Search narrows localized entry names and aliases, never account values; clearing it restores the same scopes, and no match uses `JEmptyState`. Models shows uses before a folded Advanced group. Agent groups runtime settings and session triage; Notifications and background groups mobile controls and one phone-check list; About and updates groups app update actions; Connection groups status, address, pairing and diagnostics. These groups reduce navigation choices without changing behavior defaults. Old destinations remain reachable. See `docs/android/android.md`, "Settings navigation and search" for the entry map and capability limits. This requested structure change is a large UI change requiring human screenshot review.

### 3.6 Sheets

- **Live recording sheet** (#4240, `RecordingLiveSheet`; full screen and dark while it records since #8731, "The meeting room" below): a screen with its own navigation stack for something that keeps running when it is dismissed. It opens by itself when the activity starts, **Hide** or a swipe leaves it, and a one-row card on the page is the way back. After the activity ends the same sheet cross-fades to a wrap-up of what happens next, so the user is not dropped on a list. Reason: a recording is the one task whose controls must stay under the thumb while the user looks at what is being heard, and whose end needs visible follow-through (saved, transcribed, synced, minutes) instead of a word in a list row. A room meeting's sheet (#3862, a phone meeting recording started while the hub listed `meeting_conversation`) shows the meeting page in place of the live view, and after Stop the same page goes on as the meeting's thread in place of the wrap-up ("Meeting page (#3862)" below); ordinary recordings and #3746 meetings keep this sheet's look.
- **Consent sheet** (#4741, `RecordingDayConsentView`): before something that records other people or sends their words to a model starts for the first time. A large-detent sheet with an inline title: the mode's mark, a heading that says what is about to happen, then one row per point (a `ThemedIcon` in the accent beside the words, read as one element), what the user can do later in a caption, and **Agree** (prominent, the one solid button) over **Not Now** (borderless) pinned at the bottom in a safe-area inset, so both stay in reach at every text size while the points scroll. Agree is the only way to agree; Not Now and a swipe down change nothing. The agreement is stored with the sheet's version, and a new version asks again. The points come from one list (`RecordingDayConsentPoint`) the tests count, not from the view.
- System `.sheet` with detents for most things (43 detent sites). Settings is a near-full sheet with a close button.
- The composer's + panel on the iPhone (`AttachSheet`) is a system sheet with detents medium and large and the drag indicator, like the system's photo picker; its bar sits in `safeAreaInset(edge: .bottom)` so the keyboard pushes it up (#4312).
- Link card: detents 0.6 and large, background stays interactive down to 0.6 (`apple/Persona/Views/Link/LinkSheet.swift:142-146`).
- **Bar buttons of a presented page** (`apple/Persona/DesignSystem/SheetActions.swift`, #9653): `SheetClose` is the ✕ at `.cancellationAction`: a close-role button with the style's ✕ (`ThemedIcon(Theme.Icon.close)`) in the primary ink, named Close for VoiceOver, closed by Esc from a keyboard, identifier `sheet-close` unless the page gives its own. The system's own ✕ (`Button(role: .close)` without a label) takes the app's tint, which read 2–3:1 on the bar's glass in dark. `SheetConfirm` is an edit sheet's ✓, the system's confirm button (`Button(role: .confirm)`, a white tick on the accent, at least 3.5:1 by `PersonaPalette`) at `.confirmationAction`; a verb that says more stays the page's own item there. Edit sheets reach `SheetClose` through `sheetCancel(unsaved:)`, which asks to discard on Mac only; on iOS it leaves at once. A page that only shows things and is also pushed elsewhere takes `sheetDone`: the ✕ as a sheet, nothing when `detailPresentation` pushed it into a column (a to-do's details). A self-drawn header (the link card, a hosted chat, the terminal's grab bar, the image viewer) puts its ✕ first, before its title. Pages never write their own close or put one at the trailing edge; `SheetCloseTests` counts both. Before iOS 26 (no close or confirm roles) the ✕ is a plain button with the same icon, and the ✓ is the word it stands for (its name, or Done), as iOS 18's own sheets have it: a ✓ there read like the selection ticks of the form under it.
- A word on a sheet's bar that only closes draws in the primary ink, `sheetBarLabel()` (`apple/Persona/DesignSystem/SheetActions.swift`, #9460): on the bar's glass, which turns lighter in a dark sheet over a light page, the accent read 2.1:1 (Send To) and 3.1:1 (Forward To), and 2–3:1 across the app's 54 bar Cancels (#9634). `SheetClose`'s ✕ replaced those words; a confirming button keeps the accent.
- Busy sheets disable interactive dismissal. Detents and drag indicators are inconsistent (DS-22).
- Confirmation sheets for Close and Delete agent exist because a system dialog was too wordy (`apple/Persona/DesignSystem/Components.swift:444`).
- Web: dialog is a centered card, becomes a bottom sheet on phones. Android: `JSheet`.
- **Call screen** (#9290, `CallScreen` in `apple/Persona/Calls/CallScreen.swift`, its controls in `CallControls.swift`, the assistant in `CallAvatar.swift`): a system call (on the iPhone the composer's phone icon in any conversation, #9887; a redial from the Phone app's Recents) is a full-screen page over the whole app, as the Phone app's in-call screen, in the call's own window above the app's (`CallWindow`): it slides up over whatever page, sheet, full-screen cover or + panel is open and dismisses none of them, and Minimize goes back to it. Reason it is not a sheet: a call is one task that goes on while the person talks, with its controls always under the thumb, and a sheet's detents and swipe down would hide it by accident; minimising it is a choice, and never ends the call. While it is up it takes the keyboard's focus and VoiceOver reads only it, as a full-screen cover would. Laid out as the Phone app's in-call screen (#9887 polish): **Minimize** at the top (`Theme.Icon.callMinimise`; VoiceOver's escape does the same); in the top third the assistant, its name and right under the name where the call is, then its latest words in the room left; the round controls low, in a row with even gaps, and the red End centred at the bottom. The assistant itself, its own look as the app shows it elsewhere (`CallAvatar`: `CallAssistantDot` at `Theme.Size.callAvatar`, Mochi drawn or a package's or an uploaded picture's frames, as the Persona tab's dot; the look the call was made with, kept for the whole call beside its name, its frames decoded off the main actor and the built-in drawing until they are): it shows each state of the call (#9887, `CallAvatarLook`, the table in docs/apple/calls.md "The avatar"): idle and blinking while it listens or the owner speaks, its thinking picture between the owner's words and its answer, working while its conversation works, needs-you while a card waits for the owner, still with its eyes shut (Mochi) or its idle frame dimmed (a package, an uploaded picture; `Theme.Call.heldPicture`) on hold under a pause mark in `Theme.caution` with the ring in caution, still under a slashed microphone with the ring gray while muted, asleep and greyed once over (`offline`). While it speaks every call shares one speaking layer, whatever the picture (Mochi, a package, an uploaded photo, an Agent's mark): a gentle bounce with the voice's level (`CallSpeech`: a scale of 1.00–1.04, `callAvatarBounce`, and a lift of `callAvatarLift`; never warped), the ring brightening with it, and a soft ripple leaving the avatar's edge on each peak of the voice (`callRipple`, `callRippleGap`); Mochi also moves its mouth (`PersonaFace.mouth`, a call-only parameter whose default leaves Mochi's drawing as everywhere else) and a package plays its idle frames under it. The look's own state pictures and expressions are the main conversation's only: an Agent's mark, and another conversation's look, stay the same picture, with the shared layer, the ring and the marks. A thin ring around it follows the voice's level (the microphone's while it listens, its own voice, brighter, while it speaks), an arc laps it while the call connects; one clock, the ring's own, drives the ring, the ripples and the avatar's scale and lift, and the picture is not drawn again for them (Mochi's face alone, as its mouth moves); Reduce Motion and Low Power Mode (followed as it is switched) hold every picture and the ring still, with no bounce, ripples or mouth, a bright still ring saying it speaks, and the clock stops while the avatar is not seen (off screen, under Call Details, the app away); the avatar stays decorative, and VoiceOver hears what it shows as the state line's value beside the running time (Listening, Thinking, Working, Waiting for your decision…), a decision waiting announced as it comes; the voice's own pause (its upstream paused while nobody speaks) is the call listening, never the hold look; a tap while it speaks stops the voice, as the voice bar's orb. Under it the assistant's **name** (as the main conversation's top bar says it, `AppModel.assistantShownName`: the current assistant's on a hub with several, else the look's, "Persona" for the built-in; what the system's call UI and Recents show, `PersonaCall.name`, #9887); for a call to another conversation (#9887, `PersonaCall.conversation`) its **title** instead, wrapping to two lines (three from xLarge up), shrinking a little first and then cut at its end, with a secondary line under it saying whose it is (the agent and its machine for an Agent session, the assistant's name for another assistant conversation; one line, cut in the middle), and in the assistant's place an Agent session's own mark (`CallAgentFace`: its runtime's `BrandMark`, or its letter, at `callAgentMark` of the well's round disc, still; the ring around it as for the assistant), the line of where the call is right under the name (Calling…, Connecting…, the running time in monospaced digits, On Hold, Reconnecting…, Call ended · why, which wraps whole however long the reason) and, a section's gap below, Persona's latest caption, in the room left over the controls (at least four lines', more where the face leaves it: a long answer reads whole at the default size), cut at the end, so the call stays useful when looked at. Nothing is lost: VoiceOver reads the name, the secondary line and the caption whole, and a tap on the name or the caption (or VoiceOver's **Call Details** action on them) opens **Call Details** (`CallDetailsSheet`), a sheet over the call screen with the whole name, whose it is, the call's line and every caption of the call (`VoiceCaptionList`, as ⋯ › All Captions lists them), opened on the name or on the newest caption by what was tapped; Minimize and the end of the call take it away with the page. At the bottom the round controls (`CallControl`: a `Theme.Size.callControl` disc and a one-line caption under it, `buttonLabelLine`): **Mute** and **Speaker** in a row as toggles, filled in the shell's ink while on (`Theme.Shell.ink`, its quieter grey `roundMuted` while off, as the dictation bar's round buttons) and read as selected; **Audio** in Speaker's place while Bluetooth, a car or headphones can take the call (the system's route picker, `AVRoutePickerView`, its own glyph clear over the style's disc, its value where the sound goes now); **Resume** beside them when the call is held and its audio did not come back; **End** in `Theme.danger` under them (where the three do not fit one row, a narrow iPad window at a large text size, Mute and Speaker over Resume and End, as the urgent call's page). Minimize and the controls always keep their room: the assistant gives way first (to `callAvatarShare` of the page, then to what the words leave it, down to the voice bar's orb's size, then altogether: the words say where the call is), the caption keeps its least room (four lines) so nothing moves as words come, and the face sizes itself by that room, never by the words in it (once the call is over the caption and its room go, and why it ended has the room), and where the words alone do not fit the middle scrolls; the name, the line and the controls are never cut at any size. Every control acts through `PersonaCalls` (never on the voice itself), so the system's call UI and AirPods say the same. When the call ends the line says why for two seconds (the call stays that long), then the page goes. An answered urgent call's page (`UrgentCallView`) keeps its matter, card and captions, its header showing the assistant's small face (`callBarAvatar`), its name (the call's own, as its handle says it) over **Urgent Call**, and the running time; under the header everything scrolls between it and the controls, the call's title first (whole, however long: it was three lines, cut, at the larger text sizes), then the matter, the card (its options wrap, centred in their buttons; a permission's summary wraps whole) and the captions; under them the same round controls for Mute, Speaker or Audio, Resume and Hang Up (in one row while they fit, else Mute and Speaker over Resume and Hang Up), over them its line (wrapping), saying On Hold or Muted when it is; like a call to Persona it starts on the earpiece (`docs/assistant/urgent-call.md`). The interface is capped at xxxLarge like every screen (6.2), and the page and the return bar take Settings › Text Size as the pages do although they are drawn outside the app's root view; the captions here are a glance, the whole of them is reading content in the conversation.
- **Return-to-call bar** (#9290, `CallReturnPill` in the call's window, `CallWindow`): while a call to Persona is on and its page is minimised, a capsule at the top of every in-app screen, under the status bar: the Assistant and Agents pages, any conversation, Settings, and over sheets, full-screen covers and the + panel too (its window is above theirs, and takes touches only on the bar). Pages lose nothing under it: the app's root gets its room (`additionalSafeAreaInsets` on the window's root, so every page, the shell's UIKit-hosted top bars too, starts below it; a SwiftUI `safeAreaInset` at the root did not reach those bars), and so does every sheet or cover whose content would start under it (as much more safe area at its top as reaches past its content's top; a sheet lower down, the + panel at half height, an iPad form sheet, needs none; nor does an alert or action sheet, which floats clear of the bar, and room at its top would push its words down under its buttons); the toasts get it too, so a toast stays just below the top bars the room moved down. UIKit says nothing when a page presents another, so while the bar shows it looks again four times a second, in front only: the watch is cancelled as the app leaves the front and the room put right at once as it comes back. The assistant's small face (`CallBarFace` at `callBarAvatar`: the call screen's avatar in the same state but still, #9887: Mochi's still drawing of the state, a package's first frame of it, dimmed on hold, greyed once over; the call's own look, never one chosen during the call, its frames decoded off the main actor; an Agent session's mark for a call to it, #9887; no motion and no clock, since the bar sits over every page for the whole call and at its size a bounce would not read), then **<name> · 03:12** (the conversation's title for a call to another conversation) (Calling…, Connecting…, On Hold, Reconnecting… in the time's place; the time is never cut, the name takes what is left and is cut in the middle, `CallBarWords`, and VoiceOver reads it whole), a slashed mic while muted, a pause mark in `Theme.caution` on hold, on the style's surface washed with the positive role's soft tint (`Theme.softTint`, the role of a call that is on), at least `tapTarget` tall. A tap opens the call screen again, over whatever is open; so does coming back to the app with the call on (the Dynamic Island, the status bar's call, the Lock Screen's call, the app switcher). Hiding the page, another page, another app, Home and the Lock Screen never end the call. In the main conversation the voice bar stays in the composer's place for it, with **Show Call** (`Theme.Icon.callExpand`) beside Stop, which hangs up; a call that ended by itself leaves **Call ended · why** over the composer, whose Continue calls its conversation again (a new system call, never the old voice picked up). The voice chat in the app (the phone icon's fallback, #9887) has the same bar while it is on away from its page (`VoiceReturnPill`, `voice-return-bar`: the conversation's mark or the assistant's face, **<title> · 03:12**, its hold or mute mark), whose tap goes back to its conversation, where its own voice bar is.
- **During a call** (#9290 follow-up, `docs/apple/calls.md` "Nothing in the app ends a call"): nothing the owner does in the app ends a live call by itself. What would take its microphone is refused with a toast and opens nothing (dictation: "On a call — dictation is off until it ends"; a recording: "End the call first"); a voice chat in another conversation asks first, as an irreversible action does (section 4.4: "End the call with <name> and start a voice chat here?", **End Call and Start** in the destructive style, **Cancel** keeps the call); the call's own conversation's handset or Voice Chat shows the call screen.
- **Image editor** (#4963, `ImageEditor`, tokens `Theme.ImageEdit`): a full-screen cover, dark in both appearances like the image viewer, ✕ and ✓ (or the words Select / Add to Message) in the top bar (#9653) with Undo and Redo between them. Under the picture, a row of the shown tool's options (six ink swatches and a Line Width menu for Box and Pen; Rotate, an Aspect Ratio menu and Reset for Crop) over the tools row (Box, Pen, Crop as `.themedButton`, the one shown prominent, then Clear). Opened from the + panel, the panel's caption field and Send (`ComposerSurface`, `ComposerRoundButton`) sit under the tools. At large text sizes the tools show only their icons and the options row scrolls; at the largest, where even the icons are wider than the page, the tools scroll too and Clear stays put (#5131). No label wraps, and every row keeps the page margin. One editor for the + panel, the composer's strip and the image viewer. Reason it is not a system sheet: the picture needs the whole screen and the drags in it must not pull a sheet down. The web's (#4991, `web/src/views/chat/ImageEditor.tsx`, measures `IMAGE_EDIT` in `web/src/lib/image-edit.ts`) has the same rows, tools and order: Undo and Redo as icons between Cancel and Done; Line Width shows the chosen line itself (the web has no line-weight glyph) and its menu rows lead with each width; Aspect Ratio shows the chosen shape's name; the tools and Crop's buttons drop their words when their row has no room (measured, as `ViewThatFits`); the menus are the web `Menu`, put inside the editor's layer (`container`) so they draw over it, dark.

#### Pointer-anchored action menus

- Android `JPointerMenu` is the pointer presentation of an existing `JMenu` action list, not another action model. A regular or expanded window opens message actions at a secondary press; compact and wide-phone windows, touch long presses and accessibility More actions keep `JActionSheet`. The shell's `LocalLargeWindowInput` is the only large-window input marker; pages do not add breakpoints or device checks.
- `Modifier.onSecondaryClickAt` reports the secondary press in window coordinates and consumes that press only. Primary clicks, hover, movement and touch retain their behavior; the parameterless `onSecondaryClick` remains for callers that need no anchor.
- The pointer is the preferred anchor, growing right and down first. `JPointerMenu` measures the real menu, flips toward the other side when it fits, clamps to `WindowInsets.safeDrawing`, and leaves long content to `JMenu`'s existing scrolling. It does not substitute a centered menu, cover system bars, scroll the message or add a hover-only action.
- Escape closes and consumes only the visible pointer menu. Dismissal clears the anchor but preserves the action's existing follow-up state, so Rewind confirmation, Select and Share open and return exactly as they do from the action sheet.

### 3.7 Buttons

- One style: `.themedButton(prominent:)` (`apple/Persona/DesignSystem/Styling.swift:127`), 185 uses. Glass, bordered or soft per style. Soft buttons have a 44 pt minimum and dim to 0.45 when disabled; glass prominent dims to 0.4.
- A secondary button's label reads at 3:1 or more on its own fill, wherever the button sits (#6157). On iOS the glass and bordered (Lark) ones blend the label in the tint with the fill under it: lighter in dark, darker in light, more on a lighter fill. Plain tint read 2.0:1 on a panel in a dark sheet, where the glass turns lighter, and 2.7:1 for coral on Lark's wash in light. Soft buttons (Clay, Island) keep the plain tint on `Theme.card`. Checks: `StyleSpecTests.secondarySoftButtonsRead` for the soft fill, a token; `ButtonContrastUITests` measures the system's fills on screen, on every kind of fill the app puts a secondary button on (`-showButtonContrast YES`). Don't fix one call site with its own tint: the component already does it.
- Forms use `.formButton` and `FormAction`.
- iOS numeric extension forms use `NumberKeyboardActions` in the system keyboard toolbar: Save before the trailing
  Done uses the form's existing save eligibility, configuration-busy guard and action, and clears numeric focus once that
  save succeeds (a refused save keeps the field focused); Done only clears numeric focus. The page owns one focus value
  for all numeric fields, including grouped fields, and contributes one toolbar. Native bar buttons use `Theme.Font`,
  single-line catalog labels and the tap-target floor. Keep the form's existing Save row (`FormAction`) for editing
  without a keyboard. This keyboard-only duplicate has a separate accessibility identifier; it avoids scrolling to save
  and does not add a page-closing Done to a navigation bar. There is no Mac toolbar.
- Buttons side by side in equal widths go in a `StackingButtonRow` (`apple/Persona/DesignSystem/StackingButtonRow.swift`, #6151). It keeps them in one row while every label fits its share whole, and stacks them otherwise, the first on top and each across the row (rule 3 (c) in 1.2). It decides by measured width, not by text size: a short label keeps its row at sizes where a long one, or a long translation, stacks. Each label is a `Text(…).buttonLabelLine()`, one line that may shrink to 0.8 for a small overflow, which tells the row how narrow its button can be with the words whole. A button whose label isn't one is taken to fit. First used by the to-do page's bottom buttons; other rows of equal buttons move to it when they are next touched.
- Equal columns of values or titles go in an `EqualColumns` (`apple/Persona/DesignSystem/EqualColumns.swift`). It keeps one row while every cell's content fits its share whole and drops to two columns, the rest wrapping below, when one does not; like `StackingButtonRow` it goes by measured width, not text size. First used by the Machines card's metrics and Today's Tokens, so a number is never cut to "3.85…" at a large text size.
- Actions floating over a scrolling page's foot are pills (`.floatingPill(prominent:)`, `apple/Persona/DesignSystem/FloatingPill.swift`, #9008), drawn as the Sidebar layout's foot draws its own (#4206): the prominent one a capsule of `Theme.accent` with white `Theme.Font.bodyStrong` words, as the foot's New Chat; the other a capsule on `.surface(interactive: true)` (Liquid Glass in the glass styles) with words in the primary color, as its Search and Settings. At least `Theme.Size.avatarL` tall (46 pt, the foot's), apart from each other, the button `.buttonStyle(.plain)`. They go in a `floatingFooter` (3.12), nothing behind them but the page's blur; a row of them is a `StackingButtonRow`. First used by the to-do page.
- Icon buttons use a 44 pt frame around a smaller visual (composer round button, viewer chrome).
- Destructive actions use `role: .destructive`. Red is also used for risky-but-not-destructive actions (DS-24).
  A secondary button with that role is red (`Theme.danger`) in every style (#7042): `.themedButton()` reads the role, so a
  call site sets no tint of its own. The system's glass and bordered styles took the app's tint over the role, and the
  soft style ignored it, so a Delete drew in the accent (blue, purple in Clay, teal in Island) like any other button.
  The red label reads at 3:1 on every fill as the danger tint does (`ButtonContrastUITests` checks both).
- `SystemPasteButton` (`apple/Persona/DesignSystem/ClipboardButtons.swift`) wraps the system `PasteButton` for anything that sends what is on the phone's clipboard: a capsule in the accent with the system's own icon and word. Why not `.themedButton`: only the system Paste button reads the clipboard without iOS asking "Allow Paste?" first (the tap is the permission), and the system draws it, so it can be tinted and shaped but not restyled. Place it beside a themed button that does the same thing by reading the clipboard, with one line saying it sends without the prompt (the computer page's Clipboard section, #3064).
- Web buttons are 32 px tall (hit area expanded on touch). The web's touch target is `touch-hit` (an invisible 44 px square centred on the control); a ✕ in a tile's corner (an attachment's remove) uses `touch-hit-corner` instead (#11442), whose area grows out of the corner and only `--hit-in-x` / `--hit-in-y` (6 px by default) into the tile, so the tile's middle keeps its own tap. Icon buttons packed 4 px apart in a row (the Agents title row's Filter, ⋯ and +) take a real 44 px box on coarse pointers (`[@media(pointer:coarse)]:size-11`), as the fold-all button does (#10637), since centred `touch-hit` squares would overlap and the later sibling would take the earlier one's taps (#11511). A row that folds (By to-do's branch rows) is 44 px there, and its chevron, unpositioned, lets its `touch-hit` take the whole row; controls with their own action stay positioned above it. Android: Large 48, Medium 40, Small 32 dp drawn, each taking taps across 48 dp: Compose extends a smaller control's touch target to 48 dp unless a clipping parent cuts it off, so a frame around small controls clips with `clipDrawing`, not `clip` (DS-06; `TouchTargetTest` checks buttons, chips, switches and segments in every style).
- A group of actions has at most one solid primary button. A recommended choice has an outline and a "Recommended" tag, not a second solid button. Red is for failure and destructive actions only.

### 3.7.1 Switches (web)

- One component: `Switch` in `web/src/ui/atoms.tsx`; the assistant pages' `Switch` (`web/src/views/assistant/parts.tsx`) wraps it and only stops the click from reaching the row under it. Pages never draw their own knob and track.
- Look, iOS-style (#3675): 38 x 22 track, `--switch-on` (iOS green, `#34c759` light, `#30d158` dark) or `--switch-off` (translucent grey, 20% light, 32% dark, so the off track shows on every background); an 18 px white knob with `--shadow-knob`, a soft drop shadow with no ring. The knob never takes `shadow-btn`, whose 1 px ring reads as an outline on the track.
- Focus: the `switch` class rounds the focus outline to the pill and offsets it 2 px; it shows on keyboard focus only (`:focus-visible`), never after a click. Forced colors draw the track border and knob in system colors.
- Why green and not the accent: an on/off switch reads as iOS's, and one color on every page; the accent stays for selection and links. Pill-shaped chips that toggle a mode (botmux, find bar's match case) are not switches of this kind.

The web Recordings retention setting (#3780 / #3862) reuses the settings select and numeric input: minutes show
**Forever** by default, or a finite duration with editable days. The finite option names the duration it will save;
returning to Forever sends `0`. Only the hub-confirmed value is shown after a save, and transcript days stay separate.
This is a requested default and interaction change, not a new style or component.

The web Agent naming settings (#4812) reuse Settings › Assistant's Agents group, `Row`, `Switch`, the numeric
settings input and `SettingsHint`. The default corrects only recognizable placeholder or instruction-template
names, once per Agent; periodic updates default off. The per-Agent limit counts successful name changes, including
periodic refreshes. The periodic switch reveals hours (24 when first enabled) and a plain note that refresh needs
new output and remains subject to that limit; a limit of one shows why another refresh cannot happen. Turning off
automatic naming hides the advanced controls without losing their values. Saves use the selected assistant and
show only confirmed values. Native custom titles and manual renames are protected. This is a requested feature,
with no new visual style, component or motion.

For people who manage Agents in herdr, Settings › Agents uses the same `Row` and
`Switch` for **Use the herdr tab name as the session name** (#11576). It is off
by default and uses the hub-confirmed value. When the tab name is primary, the
sidebar and chat header keep the CLI title in the hover text, and search can
show it on the secondary line. The existing Rename dialog uses one **Name**
field for these sessions.

### 3.7.2 Tick boxes (iOS)

- `.toggleStyle(.tickBox)` (`TickBoxToggleStyle` in `apple/Persona/DesignSystem/Components.swift`): a box, ticked (`Theme.Icon.checkedBox`, positive) or not (`Theme.Icon.uncheckedBox`, secondary), beside a short label. Tapping the box or the label flips it, and VoiceOver reads it as the toggle it is.
- Only for a mark the user sets on one item among many, where a switch would outweigh the item: a file marked viewed in a code review (#827).
- `TickBox` (same file, #7013) is the same box with a third state, a dash (`Theme.Icon.partlyCheckedBox`), for a mark that can cover part of an item: a file partly staged in a session's Git tab. It runs an action instead of flipping a binding (the state is the computer's, read back after the write), shows a spinner in the box's place while that runs, and sits in a square as large as a tap target; VoiceOver reads it as a toggle whose label says what a tap does ("Stage main.swift") and whose value says the state. Why not `TickBoxToggleStyle`: a toggle has two states, and staging has three. Settings keep `Toggle`'s switch.

### 3.8 Chips, badges, avatars, marks

`Chip` (look neutral, tint or outline), `CountBadge` (99+ cap), `LinkBadge`, `Monogram`, `ChatAvatar`, `AgentMark`, `BrandMark`, `SessionAvatar`, `ThreadAvatar`. An agent named by its kind reads the same on every client, the product's own spelling (opencode and pi in lower case): `spec/fixtures/agent-names/names.json`, which the web's `agentName` and the iPhone's `AgentKind(usageAgent:)` are tested against (#10042). Chips are text-only and read as their text. Decorative marks are hidden from VoiceOver when adjacent text names them. Sources are in `apple/Persona/DesignSystem/Components.swift` and neighbors.

The web has the same `CountBadge` (`web/src/ui/CountBadge.tsx`, #4538) in three tones: `needs` for what waits for the user (for now the web's other Needs-you counts' look, `--attention` on the neutral field as the sidebar's badges; #4653 turns all of them to the Needs-you blue at once), `neutral` for a passive total, `accent` for a selected item or a mention. It shows nothing at zero and "99+" past 99, is 18 px high (16 px `dense`), and its exact number is spoken unless the host control's label already says it (`label={false}`). The Inbox section bar (#9053) uses it first, so it is a module of its own, off the first screen, while only that page draws it; the web's other counts keep their own pills until #4538 moves them.

Apple counts and row marks (#4538): `CountBadge` takes the web's three looks by name. `.needs` (Needs you's text on its soft tint) is for what waits for the user, `.neutral` (white on `Theme.countBadge` gray, the default) for a passive total such as unread messages or items in a group, and `.accent` (white on the accent) for a mention or the selected item. It shows nothing at zero and "99+" past 99 (`CountBadge.text`, which the tab and pane badges that must be plain strings also use); VoiceOver reads the exact number unless the host passes its own `label`. A tab bar's count is the system's: white on red (`Theme.tabBarBadge`) at its item's symbol's top trailing corner, the iPad's tab bar and iPhone Duo's (#4231, `TabBarBadge`); the same number the row and the list's Needs You show, only drawn as a tab bar draws it. On iPhone Duo's tab bar it is `TabBarCount`: `Theme.Size.badge` (18 pt) tall, round for one digit and a capsule for more, its number `Theme.Font.labelHeavy` (12 pt) white, ringed `Theme.Shell.tabBarBadgeRing` (1.5 pt) in the page's color so it stands apart from the chosen item's circle and the symbol, and centred `Theme.Shell.tabBarBadgeInset` (2 pt) inside the top trailing corner of the symbol's 32 pt well (`tabBarBadgeSpot`): its centre is off the circle and a third of it at most lies over the circle's edge. Centred on the corner itself its ring reached into the name of the item above; inside the corner, as before 11-17, it lay over the circle and most of the symbol. A dense row's state at its end is `RowStateMark`: Needs you a still blue dot, Unread a green dot, Working the amber spinner (`WorkingIndicator`), nothing otherwise, used by the Assistant sidebar's rows (the main chat and the Directions) and by All Chats (`AutoConversationsView`). Why: the sidebar drew Unread in the accent, the same blue as Needs you, so the two states looked alike, and its Needs you counts were white on solid blue while every other Needs you count was blue on the tint. The To-dos list's Working mark (iOS, hubs with `todo_view`, from #10465's switch-on) is `WorkingArc` instead, the same amber turning: a Core Animation arc at a text mark's size (`markXS`, scaled) that stops off screen, on another tab and with the scene away (`VisibleWork`) and holds still with Reduce Motion, which the system's mini spinner does neither of, on a list that can hold many working rows (proposal 3.4).

**Pictures of people and conversations** (#10923, `apple/Persona/DesignSystem/ChatAvatar.swift`, `docs/protocol/protocol.md` ("Avatars (#10923)")): `ChatAvatar` draws the picture an item names by the hub's avatars route (`avatar_url`) over the name's `Monogram` of the same size. The initial is there at once and while the picture loads, stays when there is none or it cannot be loaded, and the picture takes its place without moving anything (same size, a circle with a 6 % hairline, as `LarkAvatarImage`). It asks at the smallest size the route draws (64, 128, 256, 512 px) that covers the points drawn times the display's scale, through `LarkMedia`'s memory and disk caches, so a scrolled-back row never asks again; hidden from VoiceOver (the row names whose it is). Without a picture it is pixel for pixel the `Monogram` the row drew before. The Inbox's connector rows use it first; conversation pages and other lists move to it as their sources move to the route (`ThreadAvatar`, `LarkAvatarImage` until then). The web has the same in its Inbox `Monogram` with `src` (`ItemAvatar`: the path asked at the pixels drawn, `?size=` the size × `devicePixelRatio`, rounded up by the route), the letter on error. Android's `MessageItem` and its loader share `MessageItemTokens.avatarSize` (42 dp for Needs-you, 38 dp otherwise); `LarkAvatar` keeps its 40 dp default. `rememberLarkAvatar` converts that drawn size to display pixels for the hub request and Coil decode, with separate decoded-cache keys per scoped ref and pixel size. Deferred reads and both cache layers retain a non-default manager through `LarkContentSource` (#10932). Why a new component: one loader and one cache for every source's picture, where each source had its own.

**Brand lockup** (#9468, #9508, #10172; `apple/Persona/DesignSystem/PersonaBrandLockup.swift`): wherever the app signs a picture. One component in two sizes: `.mark`, the quiet signature at the foot of an exported picture (the current assistant's face (`PersonaAvatar`), its name and "(Persona)" after it, "Persona" once when that is the name); `.banner`, what a chat's screenshot shows where its top bar is (`PersonaBannerRow`). The banner has the face (`Theme.Size.avatarM`) and the name (`display`) at the leading page margin, and GitHub's mark (`BrandMark`, `Theme.Brand.github`) with the repository (`PersonaBrandLockup.repository`, the one place the app spells out `nwverse/persona`) at the trailing one: one type level below the name (`title`), in the secondary color, mark and words alike, centred on the name. The repository goes last: short of room the name is cut at its tail and the repository stays whole; then the banner turns slim (the face at `wellXS`, the name in `heading`, the repository in `secondaryStrong`; also a folded header and the bar's largest text sizes); only where not even the slim row has room for the name's first two characters and the ellipsis beside it, the mark stands alone. A picture, not a link. Bar sizes stop at `Theme.Shell.topBarLargestTypeSize`. **Screenshot banner** (`screenshotBanner` in `apple/Persona/DesignSystem/ScreenshotBanner.swift`, docs/apple/ios.md "Screenshot banner"): a bar that is on screen but out of screenshots, recordings and mirroring, with the banner under it; an opaque backdrop under both, so neither shows through. Only on the chats' own bars (a system navigation bar cannot be moved into that layer); Settings › Appearance & Display › Persona Banner in Screenshots turns it off, and off the bar is drawn as it always was. Why a new pattern: the user asked for the brand in shared screenshots without changing the screen.

**Info tips** (#6458): an ⓘ beside a title that says in a sentence or two what the thing it names holds, when its name does not say and the words would crowd the list if always shown. Web `InfoTip` (`web/src/ui/InfoTip.tsx`): a 16 px button with an 11 px `info` mark in `--ink-3` (`--ink-2` on hover or while kept open) and a 44 px touch target; the pointer resting on it for 250 ms, keyboard focus or a click shows a dark tip under it (above when there is no room; `--tooltip-bg`, `--tooltip-fg`, `--tooltip-border`, 12 / 18 px, 260 px wide at most, drawn over the page so a scrolling list never clips it); a click keeps it until a press elsewhere, Escape, a scroll or another click; its words load on first use (`InfoTipPanel.tsx`), and the button names them (`aria-describedby`) while shown. It never carries `aria-expanded` (a sidebar group heading's buttons with it are the group's fold). Apple `InfoTip` (`apple/Persona/DesignSystem/InfoTip.swift`): `Theme.Icon.info` in the secondary color on a `Theme.Size.tapTarget` square; a tap shows the words in a popover (`presentationCompactAdaptation(.popover)`, at most `Theme.Size.box260` wide); spoken "About <name>" with the words as its hint. The same words are the empty state of an open group with nothing in it.

**Persona's signature** (#9468, `PersonaBrandLockup`): wherever the app signs a picture it makes, the assistant's face (`PersonaAvatar`), its name (`AppModel.assistantShownName`, label semibold, secondary) and "(Persona)" (label, tertiary; left out when the name is Persona). One component for every such place: `.mark` at the foot of an exported picture, `.banner` over a screenshot (the brand lockup above, with GitHub's mark and the repository in place of "(Persona)", #10172; its name is the top bar's, `AppModel.assistantTitleName`, #12243). Never a lookalike of it assembled in a page.

App icon badge (#5824): the number on the app's icon (iPhone home screen, Mac Dock) is the sum of the
tab bar's count badges (Persona, Agents, Inbox), computed by one function (`AppIconBadge.count`), never a count of its
own. A new tab badge joins the sum only if it counts what waits for the user or is unread; a passive total (open
to-dos) or an extension's badge stays out. `docs/apple/ios.md` "App icon badge (#5824)".

"On another computer" (web, #5421, cindy's `RemoteSignalOverlay`): `AgentMark` `remote` adds one arc and a dot off the mark's upper right, in the mark's text color, overhanging its box without taking room (a 16-unit canvas whose lower-left 12.5 units are the mark). The web chat's top bar draws every Agent's mark with it, since an Agent always runs on one of the user's computers: the mark's hover and accessible name say "Agent runs on {computer}", and while that computer is offline the mark is at 75 % opacity and says it continues once the computer is back.

Web chat top bar (#5421, the owner's requested alignment with cindy's `SessionContentHeader`, 2026-11-12; `docs/web/web.md` "The top bar"): one line. At its start, in order: the pin mark and the loop mark (13 px, `--ink-3`, only when they apply), the Agent's mark (14 px, `remote`), the title (14 px medium, truncating; a double-click edits it in place, Enter or blur saves, Esc cancels), ⋯ (28 px, 15 px icon, `--ink-3`) and the git branch (12 px, `--ink-3`, from 800 px of header width, not interactive). At its end, the side panels' buttons (Background tasks, Team, Side sessions, Files) and the one button to the other view. It shows no state, model, permission mode, subtitle, goal or space button: the state is the running status bar's and the row's, the mode the composer picker's, the goal a bar over the composer. The bar stays 56 px tall (cindy's 46) so it lines up with every other page header and side panel header. A Persona conversation keeps its face before the title and the same order. In a task's or an Agent's side panel (#5424, while Settings › How Agents open says Side panel) the same bar is the panel's only header: the task's title in place of the Agent's (its state and place in the title's tooltip, no in-place rename), a task with several Agents adds ⌄ to switch after the title, ⋯ starts with the panel's own rows (Open in new window, Remove task…), the same space's other Agents are left out, and × at the end closes the panel. Before ×, the chat bubble (`chat`, the icon the agent page's view button shows for Chat; "Open Agent page") opens the Agent shown as its own page with the way-back strip and closes the panel; it is absent while the task has no Agent up.

Assistant avatar: a disc with the first character of the assistant's name. The default assistant's is the AI purple; any other takes one of five hues (blue, green, orange, pink, indigo, in that order) by the sum of its id's UTF-8 bytes modulo 5, so every client colors the same assistant alike. A status mark sits at its foot (online green, offline neutral, installing orange) and, on the sidebar's head only, a mark at its head when another assistant has something waiting. Web: `web/src/ui/AssistantAvatar.tsx` with the `--ai` and `--hue-*` tokens. The letter is sized by the disc, not by the system's text size, so it never outgrows it. Android: `JManagerAvatar` (the status mark's ring is cut out of the disc, so the surface behind shows around it), with `JAssistantSwitcherHeader` and `JAssistantSwitcherRow` for the sidebar's head and the roster's rows (an offline one's row faded with `HomeTokens.assistantOfflineAlpha`), and `JAssistantSettingsHeader` for the head of Settings' first group (the monogram at `HomeTokens.assistantAvatarHeader`, the title, and a Switch text button opening the same roster). Apple: `AssistantMonogram` (`Monogram` drawn on the assistant's hue, its letter in the panel's color and capped at the largest standard text size) with `AssistantStateDot` (ringed in the color it sits on), an offline row faded with `Theme.Fade.offline`; the sidebar's head keeps the Persona face with the same dot, and `AssistantWaitingDot` at its head. Why not the person avatar (`Monogram`, Android `JAvatar`): that one sits on the neutral surface, and here the color is what tells the assistants apart.

The task stage strip (#3380) reuses `TodoStageBar` on the task detail, home card and briefing, and iOS and Android draw it by the same rule (#11590). `stage_kind: general` omits the strip: the existing task status already describes non-code progress. Code tasks keep the six steps and `aria-current="step"`; unknown kinds retain the legacy display. Work closed with nothing left to deploy (`stage_done`, #11590) ends the strip with a Done step after the steps it went through: every dot passed (the done style), none current, the steps it never took left out rather than ticked, and never Verified, which stays the confirmed check of deployed work (#3240). The Done step reuses the passed dot, its connector and the label tokens; it adds no token. `plan` is current only for a pending plan-approval card before review, not guessed from prose or a running state. GitHub links retain `PR #N`; GitLab-style merge request paths on any host read `MR !N`, with the existing link behavior and tokens unchanged. No new setting, visual style or motion is introduced.

On iOS, `TodoStageBar` goes by the measured width, not the text size (the same rule as `StackingButtonRow`): while every step name fits its sixth of the width whole, at the normal type size and never scaled down, the bar is one element saying the stage (#5892). When one does not (HIG-027: names ended in "…" or ran together at the larger sizes), it draws its six dots and their lines without the names. One line under the dots names the current step and its place, "Step 2 of 6 · Develop", in the current name's style, and wraps if it has to; a no-break space before the "·" keeps it at the end of the first line, so the second line never starts with it (in every language). VoiceOver reads each dot in full (its name, its place, and done, current step or still ahead), and the line, which says the same, is hidden from it.

Model configuration reuses settings `Group` / `Row` and neutral `Pill` components for execution-form groups and chips
(`docs/assistant/model-config.md`, "Execution form"). Long explanatory pills opt into `wrap` on narrow rows; other
pills keep their existing single-line layout. A separate, labelled 44 px **?** control expands help inline on
hover, focus or click, with Escape/outside-click dismissal. Why inline: explanations include environment and configuration
details that must remain readable on phones and at larger text sizes; a short tooltip or a nested button would not fit.

The Main-chat agent-kind notice (#4662, `MainChatAgentNotice`) reuses the compaction strip's neutral surface, spacing,
caption text and info icon; its explanation wraps, and the quiet **New main chat…** button stays on one line, opens
the existing dialog and has the existing `touch-hit` 44 px coarse-pointer target without changing its drawn height. It appears only when the loaded launcher list confirms that the current main chat and its configured
default use different agent kinds, with no conversation pick. The Models editor uses the same rule in an inset explanation
that follows the provider draft. Same-kind, unknown and unloaded states add no warning or new action.

Assistant In progress activity marks (#3588) reuse the existing Working spinner (`TaskMark` on web, `JSpinner` on
Android) for active background commands, Monitor, workflows and subagents. A waiting parent is not an idle task.
The subtitle, not another icon vocabulary, explains what is running and its counts/phase; only a real wait with no
background activity keeps the clock. Needs you and completion precedence stay unchanged. The compact Session
projection and cross-platform rules are in `docs/assistant/unified-entry.md`, "Background activity in In progress".
This is a requested status correction, not a new style or setting; the existing reduced-motion behavior applies.
Workflow phase text may ellipsize, but the done/started count stays visible; simultaneous command/subagent counts
use another subtitle line rather than pushing the workflow count out of the row.

Agent header loop and background pill (iOS `ActivityChips` in `apple/Persona/Views/Session/SessionPanes.swift`; Android Glass `JActivityPill`, #4339 C8): one neutral pill held at the trailing end of the header's second row, as tall as the tabs beside it: the loop's arrows in the loop color with its next time (Running once due, Loop without one), a divider, and the background tasks' count by a neutral gear (whether they work or only wait is the title line's state). Its sheet holds the loop with Stop Loop (asks first, then asks the agent to stop scheduling) and each background task with View Output. The folded slim bar shows none of it.

The pill while a chat's model operates a Mac (iOS `ComputerActivityPill` and its face `DockPillLabel` in `apple/Persona/Views/Machines/ScreenViewer.swift` and `apple/Persona/Views/Assistant/Tasks/TaskViews.swift`; Android Glass `JDockPill` and `ChatComputerPill`, #4339 C9 and C18; `docs/extensions/computer-use.md` "App"): "Controlling MacBook · Click AC → 6 → × → 7 → =" in one neutral capsule, a small spinner before the words while the call runs, the screen icon once it ended ("Just controlled …"), which stays six seconds and fades. Caption-sized semibold words on one line that give way at `box280`. It comes from the chat's own transcript, the newest computer tool call (the assistant's `computer_run` / `computer_state` naming a Mac the app knows, or an Agent's own Cua Driver tools on its computer), not from a hub field. An Agent's chat puts it over the cards and the composer; Persona's conversation after its entries (iOS, in their row) or, on Android, on a line of its own under them, since a phone leaves it too little room beside them. A tap opens that Mac's live screen on iOS; Android has none, so a tap opens the work line that holds the call, where its steps come in as words (with no such line it is words only, as iOS's is for a Mac whose screen can't be shown). Brief and Board draw none.

Where a message goes in Persona's conversations (iOS `RouteChip` and `TaskMentionList`, web `RouteChip`, Android
`TaskRouteChip`; #4339 C15): while the next message goes straight to a task, one accent-tinted chip sits right above the
composer, under the action or tray bar: the send mark, "Message “task”", and a × of its own read "Hand to Persona" that
gives the message back to Persona. The route never shows only inside a tray. It is behaviour, so every style shows it,
each with its own chip (Android `JChip(accent, onRemove)`). @ lists the conversation's open tasks (not done or failed,
newest first) with their state's dot and where they run, and a pick takes the @word out and sets the same route a task's
Message sets. iOS draws them as a panel of their own above the action bar ("Message a Task", or "No tasks in this chat
yet"); Android puts them first in the composer's one @ list under "Message a task", the files and sessions after them
under "Mention a file or session", the typed word narrowing all of them, so the list takes no more room above the
keyboard than before.

New chat's @ (iOS `StarterComposer` with `MentionSource(machine:)`, Android `starterMentionSource`; #4339 C16, workspaces #2599): a
conversation that doesn't exist yet has no tasks, files or folder, so on iPhone and iPad the list offers Persona's workspaces,
then the agents' sessions on every computer (newest first, at most eight, Persona's own conversations left out) and, once a
typed word is an absolute or `~` path with a slash, that folder alone, read from the assistant's computer: folders first, then
files, names that start with the word first, hidden ones only after a typed dot. A picked folder stays typed with its slash and
the list browses it; a picked file types `@path `; a picked workspace or session waits over the composer as a context card and
goes in front of the first message as a `<persona-context>` tag. While the list is open, what needs the user over the composer
steps aside (iOS `MentionMenuOpen`): above the keyboard the lines, the list and the composer don't fit together. They fade and
fold away as the list comes in and back as it goes, in the state motion (`Tokens.Motion.State`; iOS
`withAnimation(Theme.Motion.standard)`; Android `NeedsStepAside`, the list growing in with the same spring on Glass), and the
dock keeps the page's own backdrop meanwhile, so the things to ask fade out above it instead of being cut at the composer.
Same rows and words as a chat's @ list ("Reading folder…", "Nothing matches in this folder" while browsing); every style draws
it in its own list, since it is behaviour.

A chat's @ list (iOS `ComposerMentionMenu` with `MentionSource(machine:cwd:files:)`, Android `mentionRows`; #4339 C16) reads
its session's computer: after the files its tools used it lists the session folder's own entries (folders first, then files,
hidden ones only after a typed dot, at most twelve, none a used file already is; "Folder" for a folder and "Session folder"
for a file beside the name), then Persona's workspaces on iPhone and iPad, then the other sessions; a word with a slash lists
that folder alone, relative to the session's, and a picked folder keeps the list browsing it. The + menu has the same list's
first step, "Mention a file or session", in every composer that has an @ list: it types the `@` at the caret, as the @ button
beside + does.

Workspace rows (#2599) are the existing resource row with the folder mark, title and a second line computer · shortened folder
(· branch), at most eight after the folder's entries and before sessions. They match title, raw folder, computer or branch,
case-insensitively; title prefixes come first, then available before unavailable, stable in the hub's order. The first row for
one computer and folder wins (trailing `/` ignored), and folderless rows are omitted. Unavailable rows stay pickable. Persona's
conversations and the ordinary new conversation read `GET /api/v1/workspaces` once per opening; Agents, Health, Mac and
visionOS don't offer them. The active hub link holds only its last good in-memory snapshot: later openings show it before
the request answers, a different answer replaces it, and an old hub or unavailable manager keeps it. A hub that has never
answered has none, so only that first opening can shift when the response arrives; there is no prefetch. Removing the hub
group drops the link and its snapshot. A pick takes the @word out
and adds a removable Workspace context using the existing `QuoteBar`; send writes `<persona-context kind="workspace"
machine="…" cwd="…" branch="…">` before the message (`branch` when known), which the manager expands as for the web. No new
component, token, style or setting is introduced. The separate legacy BeautifulUI `PromptComposer` @ menu is unchanged.

Agent header permission mode (#3719, `HeaderModeButton` in `apple/Persona/Views/Chat/SessionModes.swift`): an icon-only
capsule at the trailing end of the open Agents header's second row, under `⋯`, as tall as the tabs, with a 44 pt target that
overhangs the row. It draws the mode menu's own symbol (one per mode) in its `ModeTone`, tinted as `ModeChip` tints a
mode that is not the quiet one; Ask First is the quiet gray, never hidden, so the control stays in one place. A mode not
reported yet is the plain permissions symbol, never a guessed Ask First. The words are in its menu (headed "Permission Mode: …" as on the web and Android; just "Permission Mode" while no mode is reported, as the capsule says it, `SessionModes.modeHeading`, #5596;
then the mode list, current one checked, Allow Everything disabled with its reason where the agent cannot take it), its VoiceOver label
and the pointer tooltip. Going into Allow Everything asks first; so does leaving it when the agent cannot come back
(`SessionModes.confirmation`), on the header and the composer's model menu alike. An agent whose mode can be neither
read nor changed shows none. The folded bar shows no marks, only ⋯ on its right, as wide as back, so its capsule stays centered. Why a new
pattern: Claude Code keeps the mode in its footer at all times; the composer's chips (`ModeChip`) are a level down on
the phone. Android (#3957) does the same; its header is one row, so the button sits left of `⋯`, and it has no folded bar.
The web's header has no mode button (#5421, cindy's top bar): the mode is the composer's permission picker (#5324, with
cindy's symbols: a raised hand, code, a list, sparkles, a warning triangle; `pickerIcon`) and Session info; iOS and
Android keep their header buttons until they follow (#5432).

Full access by a restart and `/permissions` on iOS and Android (#6513, the web's #6476): where the node restarts a terminal
Claude Code into Allow Everything / Full access, the mode menus enable that row with the restart note under it (the
confirmation repeats it). While the restart waits (`bypass_pending`), the mode buttons keep the mode the agent runs in, so
nothing is drawn before it happens, say the session restarts when the turn ends (on iOS in the header menu's title,
VoiceOver's hint, the pointer's tooltip and a line over the composer menus' modes), and picking the checked mode sends it
without a confirmation so the node calls off the restart directly. `/permissions` in the composer's "/"
list opens the mode list with all five modes, nothing sent. iOS can't open a `Menu` from code, so there it is
`PermissionModeSheet` (`apple/Persona/Views/Chat/SessionModes.swift`): a full-height grouped form (a medium detent cut
off Allow Everything's note in English) of the same rows
(symbol, name, what it does, a check on the current one, Allow Everything in `Theme.danger`, a row that can't be picked
disabled with its reason), `SheetClose` at the top leading corner, and a footer only when Allow Everything's row does not
already carry the pending restart note; a pick closes it and goes through the page's confirmation. No new token or style.

Conversations an extension opened (#3758, `origin` `lark_agent` or `extension`) carry a source mark beside their title wherever they are a row, in the slot and size of the channel mark (iOS `ConversationSourceMark` / `ExternalSourceMark` in `ConversationChannelMark`'s place, web `OriginMark` in `ChannelMark`'s, Android `ConversationOriginMark` in `JChannelMark`'s): Lark's `BrandMark` for Lark /agent, the extensions symbol in the secondary color for any other extension. On the web (#10358) a conversation whose opening extension is known (`opened_by`, else `origin_ref`) carries that extension's own manifest icon instead, in the same size and corner as Lark's tile (`OriginMark`'s `session`, drawn by the lazily loaded `OpenerTile` from the installed extensions, the icon asked of the extension's own manager); while the icon loads, and when the extension has no icon the hub serves, is not installed for the conversation's manager or its icon fails, the origin's mark stays. While that icon is drawn, the mark's name and tooltip, the row's tooltip and the line under the opening message say "From <the extension's name>" with the existing "From {name}" words (the name from the installed extensions; none keeps the origin's words), and only then, so the words never disagree with the mark: loading, a fallback or a failed icon keep "From Lark /agent" / "From an extension". The extension the origin's mark already stands for, one that brings a connector or a messaging source of that brand (the Lark extension's `lark`), keeps the origin's mark and words, so Lark /agent is unchanged, while a third-party extension that reuses `lark.agent` is neither drawn nor named as Lark. iOS and iPadOS do the same: `AutoSourceMark`'s `opener` (`ConversationOpener`) draws the icon through `OpenerMark` from `OpenerIcons` (the installed extensions, read once such a mark is drawn; the icon asked of the extension's own manager on the hub's icon route and decoded off the main thread), in the box of Lark's `BrandMark` with an app icon's corner and edge (`AppIconShape`). The row's VoiceOver label (the sidebar's and All Chats', `sourceLabel`), the mark's and `ExternalSourceLine`'s words follow the drawn icon in the same way, with the same fallbacks. Android does the same in the drawer's rows (Recent, 常驻, search results and All chats): `ConversationOriginMark` takes the row's `ConversationOpener` and draws the icon through `JSourceMark`'s `picture` (in the box of Lark's inline mark, with the source tile's corner, on `catalogLogoBackground`) from `OpenerIcons` (the installed extensions, read once such a mark is drawn; the icon asked of the extension's own manager on the hub's icon route and decoded off the main thread); its TalkBack name follows the drawn icon in the same way, with the same fallbacks. The Android opening message uses the same icon and words in the channel line’s layout (`JSourceLine`). A message with a channel of its own keeps its channel line. A row has one source mark: the origin's replaces the channel's. No text tag: the row's accessible name carries "From Lark /agent" / "From an extension" instead. The message that opened the conversation gets the "From …" line channel messages already have (iOS `ExternalSourceLine` beside `ChannelSourceLine`, web the channel line in `SegmentView`, Android the bubble’s source footer), on that first message only and only once the page has loaded back to it; the chat's header stays as it is. Why not a new badge or a pinned line: the owner chose the existing channel patterns so the same place says the same thing for a channel's message and an extension's conversation.

The web main conversation has one compact header line on the welcome page and in the open main chat (#4572).
It uses the same neutral text in both places and reveals the full line on hover, focus or tap when space is tight.
The summary describes the observed harness and execution environment. The new conversation's launcher remains a
separate composer setting. A missing execution report, a proxy node and the last known location each have distinct
copy; none is presented as the current location.

**Model pill and unified model panel** (web, #5032; `docs/web/web.md` "Model pill and model panel"). This part follows
Cindy's model selector at the user's request (2026-11-12): where an earlier rule here differs, this entry wins for the
pill and panel, built from Persona's tokens. Who it serves: people working with coding Agents, and everyone in a Persona
chat (#5033), where the panel only offers models and says plainly which one the chat uses.

- Pill (`web/src/views/chat/ModelPill.tsx`): 30 px tall capsule, 10 px side padding, 4 px gaps, 13 px text
  (`ink`), no frame at rest; hover and open draw a 1 px `--line-strong` border on `--hover`. Contents in order: the
  mark of the Agent the launcher runs (#5497: `AgentMark` at 15 px, `ink-2` for the one-colour marks, 4 px before the
  name; an Agent with no mark of its own gets the generic `agent` icon, never a letter, unless its runtime names it
  (#8142), when it is drawn as the Agents list draws it), the model name (truncates), the
  effort label 4 px after it without a
  tier colour (English "Extra" for "Very high"; the full name on hover), a filled bolt for Fast and the workflow
  `UltraMark` (`ink-2`), a 14 px chevron (`ink-2`). Widths by the composer card (`@container/composer`): ≥ 600 px 72–320 px;
  < 600 px the width its words need (name ≤ 108 px without a trailing bracketed qualifier, a context size kept as a short
  mark, "Opus 5.5 1M"; the whole name in the tooltip;
  shrinking only when the row is short, the name truncating first, the effort only once the name has no room); < 520 px 12 px text, marks a step smaller; < 420 px 64 px, the
  mark and chevron only. States: loading (a 13 px `Spinner` and "Loading model…"), failed (the `warning` icon in `--red`),
  pending (#5035, a switch to another launcher that the next message carries out: a 12 px clock in `ink-3` first, then
  the target engine's mark and the launcher's name in `ink-3` up to 96 px, hidden under 600 px, before the target's model and
  effort; the engine's name in its place when the engine changes; the tooltip "Current: …" over "Next message: …"),
  disabled (50% opacity, no pointer), and a label (not a button) where nothing can be switched. The click keeps the caret
  in the message box.
- Agent launcher label (#10248): when the session reports its launcher, the existing pill says **launcher · model**,
  using the same launcher name as its model-panel row. Both the launcher and abbreviated model stay visible at phone
  width, truncating within the existing capsule; the full names remain in its tooltip and spoken label. The existing
  engine mark stays. A queued switch shows its target launcher with the clock, while the tooltip retains current and
  next setups. An unknown launcher keeps the model-only label and compact fallback. This label correction applies to
  Agents; Persona's pill keeps its existing presentation.
- Panel while a switch waits: one warning line at the top of the list (the panel's `note`: `warning` icon, 11 px,
  `--orange`, wrapping), "Switches on your next message: the Agent restarts on {launcher} and keeps this conversation"
  for another launcher of the same engine, "Switching engines rebuilds context and may lose content" for the other engine
  (#5054); a recorded pick is confirmed by a toast ("Switch scheduled: …"). A launcher switch keeps the conversation and
  its engine, so it asks nothing.
- "Switch Agent?" (web, #5054, `web/src/views/chat/EngineSwitchDialog.tsx`, `docs/web/web.md` "Switching an Agent between
  Claude Code and Codex"): a pick of the other engine's model asks first, unless that engine is already the waiting
  switch's (Cindy's rule). Persona's `Dialog` with no title bar (named by `labelledBy` / `describedBy`), 400 px wide,
  padding 16: the title 18 / 28 medium `ink`, the text 16 / 24 `ink-2` 8 px under it, a "Don't remind me again" checkbox
  (13 px, a 14 px native box in the accent) 16 px under that, the footer 24 px lower, right-aligned with a 10 px gap:
  **Switch Agent** (primary) first, then **Keep Current Agent** (secondary), each at least 96 px and one line. A sheet on
  phones; Escape, the backdrop and Back keep things as they were. The model panel stays open under it (held below the
  backdrop) and closes once the switch is recorded. The pending pill names the engine and the target model with that
  model's own effort.
- Android engine handoff (#5302, `docs/android/android.md` “Switching between Claude Code and Codex”): Glass
  uses the existing model pill and panel. Entering the other engine asks with `JDialog`. While a switch waits, the
  pill names the upcoming engine to assistive technology and keeps its mark in the compact composer. The panel offers
  a direct cancellation above its search. In every style, the recorded handoff uses `JModelChangeDivider` and opens
  the handoff context with the existing full-text reader. Older transcript runs join the same list as read-only rows
  above the current run. Their pagination, full text and image requests retain the original conversation scope.

- Panel (`web/src/ui/model-panel/`): opens above the pill, right edges aligned, 6 px gap, 8 px from the window's edges;
  300–460 px wide with a 428 px scrollable body, search on top and "Configure models" at the bottom.
  One shared column in Agent, handover, Assistant and New Agent pickers (#10249): Follow model configuration where
  supported, up to three distinct saved setups (Favorites first, then Recent), then every launcher in catalog order.
  Only the current launcher starts expanded; with no current launcher all start collapsed. Each launcher summary names
  the launcher, Agent kind, known default model, catalog model count (excluding a synthetic Default row), Current or
  configured Default status, known weekly quota and any supplied failure reason. No launcher is hidden in another bucket.
  Disclosure uses the existing chevron, the launcher's Agent mark (#5497), role colors and text tokens.
  Expanded launchers reuse model Row, family folds and Flyout, including Favorite, effort, Fast and engine controls.
  Search is flat: matching launcher summaries, then matching model rows naming their launcher even on touch screens;
  typed-ID rows explicitly name the launcher they send. Why: identical letter tiles and a separate rail hid the account
  and model choices from people comparing several launchers. The single column exposes that information progressively.
  Rows have two lines led by the Agent's mark (16 px, the launcher summaries too; the right side holds only effort and
  Fast) with hover-revealed Favorite and Customize; the model that runs has a static selected fill, no
  check. Customize opens a 264 px flyout to the panel's left: the effort slider, Fast, the engine, an exact catalog
  reference-price line where known, and a footer that says whether the setup is the recommended one, customized or
  saved. A launcher summary adds its weekly-remaining bar only when the existing launcher quota answer supplies a
  weekly ratio; its tooltip names the window and reset time.
- Provider identity on model rows (#7360): every row in New Agent and in saved setups and flat search results names
  its owning launcher in its model label or at the start of the second line, on web, Android and Apple. The rest of
  that line keeps the model ID, resolved model, description or typed-model explanation; an expanded or filtered
  launcher's own model rows may keep the description alone. A sign-in, key or model-availability error that offers a
  repair names the affected launcher in its title, and the repair opens that launcher's provider settings. This is a
  content rule, not a new badge, token or visual style.
  Preset descriptions and a truncated model's full-ID line retain the launcher name too.
  Launcher summaries retain each provider's full name as their accessible name and tooltip; the former provider rail
  and its shortened captions are replaced by the shared column (#10249).
- Model families (`web/src/lib/model-families.ts`, `docs/web/web.md` "Model pill and model panel"): inside a provider
  group, each family's name is a 11.5 px `text-submeta` line in `ink-3` (12 px side padding, 6 px above, 2 px below)
  over its rows; its right end holds the fold, a text button in the same size and colour ("Earlier versions · N" with
  an 11 px chevron, "Hide earlier" with the chevron turned up), a `--hover-2` fill and `ink` on hover, one line, its
  accessible name naming the family. The newest row says "Latest" after its name in `text-submeta` `ink-3`, no chip.
  Rows outside any family (the Default entry, unknown models) have no header. Why: one family's versions read as one
  choice and the newest is the one most people want, while older ones stay one tap (or a search) away (2026-11-17).
- Usage placement (#5056): the Agent's context meter is a separate, right-aligned row below the composer card, not part of
  the header or the model-pill container. Unknown context mounts no row. Quota bars use the established accent / orange /
  red quota tones (≤ 30% left warning, ≤ 10% or exhausted critical); price marks use neutral `ink-2`, one to three `$` by
  standard output price, and the flyout's values use tabular numerals. Unknown data never creates placeholder marks.
- Header strip (Persona chats, #5033, `PickStrip` in `web/src/views/chat/AssistantModelPanel.tsx`): one line above the
  search, 14 px side and 8 px vertical padding, a `line` rule below, 12 px `ink-2` text that truncates. It says where the
  chat's setup comes from in plain words ("Following the default · …", "Picked for this chat · …", "For this new chat ·
  …") and, when the chat has its own pick, ends with a text button in `accent-ink` (medium weight, one line):
  **Follow default** in the main chat, **Undo pick** in another. Providers that can't run Persona chats on that computer
  are the panel's last groups: the header and the hub's reason, no rows.
- Tokens: the effort levels have one colour each in both themes (`--effort-low` / `--effort-minimal` #2AAE5B,
  `--effort-medium` #14B8A6, `--effort-high` #3B82F6, `--effort-xhigh` #4F46E5, `--effort-max` #8B5CF6, Codex's ultra on
  max) and `--fast` (#3B9EFF), used inside the flyout only: the pill never colours its effort or its bolt.
- Deviations from Cindy: the Agent's mark instead of model-vendor logos (launchers of one Agent share it; #5497); price and quota appear only where the catalog / launcher
  answer supplies them; ↑↓ also move between rows; providers and engines that can't be switched to (an older hub, a chat
  that can't switch) are listed with their rows off and the reason; a pending switch to another launcher of the same
  engine names the launcher rather than the engine; the engine warning line wraps rather than truncating; "Switch Agent?"
  sits high on a computer and is a sheet on a phone, with no close ×.
- Android's existing-conversation equivalent (#4663, `ConversationLauncherChip`) reuses the composer's `JChip` and a
  `JActionSheet` with Launcher and Model sections, not a second visual selector. It reads only the hub's
  `?purpose=assistant` list. The sheet says the pick applies from the next message until changed and leaves settings
  unchanged; **Use the default** clears it. The pick follows every later send and retry in that conversation. Older hubs
  without a model list keep the launcher choices and show one disabled explanation row. No new token or style.
- Android (Glass only, #5055; Brief and Board keep their Mode sheet, launcher chips and header mode button, decision 8):
  `JModelPill`, `JModelPanel` and `JModelChangeDivider` in `android/core/designsystem` (`docs/android/android-design.md`
  2.2), from tokens: the pill `wellXS` tall with `Space.s` sides, the tile at `markS`, the name in `caption` (at most
  `box120` under 600 dp of composer), the marks at `markXS`; the same width rules, so on a phone the pill is the tile and
  the chevron. It shows while the composer's toolbar does (focused or with a draft), as Android's mode button did. The
  panel is a sheet (search, the rail, two-line rows, the strip for Persona chats) whose Customize is a page of the same
  sheet; the star and Customize are always shown (no hover on a phone) and a provider's reason sits under its group's
  name. The strip's text button is the design system's quiet `JTextButton` (`ink2` on Glass) rather than `accent-ink`.
  The header's mode button moved into the composer beside the pill (decision 11). The divider uses the model mark (no
  arrows-left-right glyph in Android's set yet). Not yet: the other engine's switch and its dialog (#5054), Codex's Fast,
  quota bars and prices, forgetting a typed id.
- Apple main-chat equivalent (#6542, `AssistantModelSheet`): the composer's existing Model row opens a system sheet with
  medium and large detents and a visible drag indicator, reusing the assistant-eligible launcher list and the same
  conversation PUT/DELETE. Only the hub-named main conversation gets it, on the computer named by `main_machine_id` (or
  `machine_id` on an older hub). If the confirmed route moves, the open conversation follows it only when the user was
  already viewing that main chat; a deliberately opened side conversation stays put. Siri, Share and CarPlay use that
  same confirmed key. The composer's current-model entry shows a disclosure chevron; its first menu row is
  explicitly **Change Model…**, with the effective model as its detail, so the conversation page itself shows how to
  reach the sheet. The macOS BeautifulUI prompt bar uses the same Change Model… action; other macOS styles use the shell composer.
  Each launcher's catalog supplies its rows; an empty or older-node 404 catalog falls back to its configured model, a
  locked launcher keeps only its valid Default choice, and a Default row sends no model ID so it follows later default
  changes. When Default and a concrete alias resolve to the same model, the concrete alias is ticked; choosing the
  current row is a no-op, never a new pick that pins the default. Resolved aliases use a lookup built when the catalog
  loads. A refresh failure keeps the last good rows beside Retry, and a provider removed from both lists is named as
  unavailable with the follow-default recovery path. An existing Claude/Codex conversation marks the other engine as
  needing a new conversation instead of offering a pick the hub would refuse. Typed IDs require `typed_models`,
  independently of `assistant_model_pick`. The sheet refreshes every catalog on pull-to-refresh; the composer's effective label redraws
  from fresh hub answers and Model Settings/assistant snapshots, never from a per-second transcript-revision poll. No new token, style, component or navigation pattern.

### 3.9 Empty states

**Web skill ZIP import** (#5478): the advanced Settings › Assistant › Skills group reuses `Group`, `Row`,
`SettingsHint`, `RowFailed`, `Pill`, `Button`, `Dialog` and `Loading`. **Upload ZIP** opens the existing dialog with a
labelled shared Button opening the native file picker. Checking is announced; the preview shows plain-text metadata and a wrapping real path.
Cancel receives focus at preview; closing restores focus to Upload ZIP. The existing dialog owns focus trapping and
Escape. Install/Undo are single-flight and cannot be dismissed halfway through a write; failures are actionable
localized text, never raw archive HTML or filesystem errors. Buttons have a 44 px target; actions wrap on narrow
screens. Semantic tokens supply light/dark colors. All content is English, Simplified Chinese and Japanese.
Explicit review-before-install is intentional even with Undo: uploaded instructions may later influence an assistant,
so selecting a file must not silently change behavior. Success refreshes the list and offers a bounded, unchanged-new-
package-only Undo with retained recovery files; `docs/operations/hub-update.md` defines the state and discovery limits.

**Web to-do images** (#11084): the existing New to-do `Dialog` takes JPEG/PNG images through the shared attachment
picker and `AttachmentTray`; selection shows local thumbnails immediately, with an accessible Remove on each. Create is
one single-flight busy action, with its failure inline in the dialog. Retry keeps every field, selected file and the
operation ID; cancel releases the previews. The to-do detail uses the same tray and existing `ImageViewer` for zoom and
download, not another gallery. Private image bytes are fetched with the to-do's credentials and manager, three at a
time, with localized Retry on failure and object URLs released when replaced or closed. The no-image dialog and detail
keep their previous layout and behavior. When `todo_images` is absent because any manager is too old to persist images,
the picker is hidden and an image-less create is never sent. All controls use the existing 44 px touch expansion and en, zh-Hans and ja
copy. The assistant manager owns persistence and handoff; the Web client does not invent a temporary upload or a local
machine path.

`EmptyStateView` (`apple/Persona/DesignSystem/Components.swift:385`): symbol, title, one-sentence message, optional prominent action. 92 uses. System `ContentUnavailableView` is used in about 22 other places and looks different (DS-15). Search results may use `ContentUnavailableView.search`. Web has no shared empty state outside admin; Android has `JEmptyState`. An offline current assistant (several assistants) takes the place of a merged list's empty state with its own: the assistant avatar faded with its offline mark, "<name> is offline" and when it last synced (web `OfflineState` in `web/src/views/ManagerParts.tsx`; Android `JAssistantOffline`, a `JEmptyState` whose `leading` slot holds `JManagerAvatar`; `docs/assistant/managers.md` "Clients").

### 3.10 Loading and error states

| Case | Component |
| --- | --- |
| Page load | `Loadable` and `LoadableContent` (`apple/Persona/DesignSystem/Components.swift:739-770`); only about 9 call sites |
| Page load (Android) | `JLoadable` and `jLoadableContent` (`Lists.kt`), the same three cases as `Loadable` plus the two a phone needs: a spinner (`JLoading`, with optional text) before the first answer; the content, kept while it reloads (`refreshing` feeds `JListPage`'s pull indicator) and with a failed reload said as one `JNotice` line above it with an inline Retry; and, with nothing to show, a `JNotice` carrying the reason (and an optional title) with Retry under it. A failed read over an empty value is a failure, never the page's empty state (`empty` slot, `JEmptyState`), so an offline cache is not shown as a successful empty answer. A retry goes back to the spinner. Retry is the shared `designsystem_retry`; the reason text comes from the page. First page on it: Settings › Hub version |
| Row or button busy | `RowProgress`, `trailingProgress`, small `ProgressView` |
| Load failure on a settings page | `LoadProblem` (`apple/Persona/DesignSystem/SettingsForm.swift:73`), no Retry |
| Saved desktop hub cannot be reached at launch | The existing OS-default local setup window keeps the saved address and per-hub session, explains the failure in plain language, and offers **Retry** plus **Local settings**; the failed first-run form is not shown as if the profile were new (`docs/desktop/desktop.md`, "Two independent installations") |
| List read failure (web) | `ListError` (`web/src/ui/ListError.tsx`): a red `role="alert"` line saying what could not be read and why, with **Retry** under it, in place of the rows; the paged session lists and the sidebar's Agents list (its complete list, and By to-do's unread to-dos) draw it, so a failed read never leaves a list blank or on "Loading…" |
| Persistent notice | `NoticeBanner` |
| A short server-driven process (web) | `Steps` (`web/src/ui/Steps.tsx`): one line per step, the current one with a spinner and `aria-current="step"`, done ones ticked green, a stop crossed red, the rest numbered and quiet. The step comes from what the hub pushes, never a local timer. Used by the alert sheet's Redeploy and by **Switch to Persona Link** on the machine detail and the admin Machines row (#10662: confirmation dialog, "Starting…" at once, then the steps, then the result in plain words; nothing shown for an older hub) |
| A short server-driven process (Apple) | `MachineLinkSwitchView`: the web's step vocabulary (one line per step, the current one with a spinner and a spoken **Current step**, done ones ticked, a stop crossed red), driven only by the hub's phase. **Switch to Persona Link** on the advanced computer detail (#10671) uses the shared `.accessibleConfirmationDialog`: the confirmation stays a dialog through xxxLarge and becomes a system alert above it, with both actions reachable. It keeps one app-lifetime start per computer across remounts, and **Check Status** after an uncertain answer refreshes the authoritative list without starting work. The unchanged hub state may offer Start again, but only another explicit confirmation posts; settled attempts expire after two minutes as on web. A running switch's disconnect suppresses the ordinary offline error and Reconnect; confirmed and unconfirmed SSH rollback are different messages |
| A short server-driven process (Android) | `StepProgress` (`StepProgress.kt`): the same completed / current / pending vocabulary, driven only by the hub's phase. **Switch to Persona Link** on the advanced computer detail confirms before one start, stores "Starting…" immediately, then follows machine-list snapshots through checking, installing and connecting to success, SSH rollback or a recoverable warning; a changed hub phase outranks that local placeholder. An uncertain answer is read back from the computer's own hub, never automatically posted again; a failed or cancelled uncertain read leaves **Check status** instead of another start, and Reconnect stays closed while that answer is unresolved. An accepted active phase older than the hub's terminal-record window also becomes **Check status**, never perpetual progress or a timer-enabled retry; only a successful owning-hub read clears it. Attempts are keyed to the pairing generation so a replacement hub cannot inherit them. Older hubs add no card (#10671) |
| Transient result | `Toast` shown by `ToastCapsule` (`apple/Persona/DesignSystem/Toasts.swift:7`) |
| Chat API failure | Web: `ErrorNotice` (`web/src/ui/ErrorNotice.tsx`), a neutral card with the red mark only and **Error details**; the error the chat stopped on is its banner over the composer with its actions (Retry, Fix it, Edit last message) and ✕, an older one the record in the chat; apps: `APIErrorCard` with details and Retry. While a model request is still retrying, the web's run status bar and Apple's `SessionLiveRetryNote` instead show the plain classified reason, observed error as secondary detail and localized current/max attempt, cleared on recovery, turn end or unwatch (#887) |
| Chat loading and waiting | iOS `ChatSkeleton` (five rounded bars of `Theme.Size.markS`, the first at the trailing edge, breathing between `Fill.well` and `Fill.chip`) while a chat's first page loads, its load error as a danger line at the chat's newest end, and `TypingIndicator` ("Claude is getting ready…", three pulsing dots) under your message until its turn's work line comes; Android `ChatSkeleton` and the error line on Glass, `TypingLine` in every style (#4339 C9) |
| The day over a message | iOS `DayLine` (`Theme.Font.caption`, tertiary, centered): over the first line of each day in Persona's conversation, never in an Agent's chat, and over a first message the transcript does not have yet (the first-message page, a conversation with an empty transcript), so the day does not appear above it later (#1911); Android `DayLine`: the transcript's in every style, those pending ones on Glass (#4339 C18) |

Toasts: error, success or info; one slot, so a new toast replaces the old. Duration 2.5 s, 4 s for errors, 5 s with an action (`apple/Persona/DesignSystem/Toasts.swift:57`). Drawn in a pass-through window and announced to VoiceOver. About 255 call sites. Loading and error shapes are inconsistent across screens (DS-15, DS-16). Web toasts are dark capsules at the bottom (2.8 / 5.2 / 6 s). On a narrow web screen (#11442) they sit in the part the soft keyboard leaves visible and rise clear of the chat's dock or the open sheet's footer (`--toast-lift`, `web/src/ui/toast-lift.tsx`), so a toast never covers a control; only a mouse pointer holds one, so a tapped toast still runs out, and an action's touch target meets the close button's instead of being covered by it. Android draws the same toast (`JToastHost`, one per window: the root's, and the image viewer's) and keeps system toasts for places where no Persona screen shows (DS-25).

**Error notices** (web, #5157, #5313; cindy's `ErrorNotice`, `ErrorBanner` and `ErrorMessageCard`, which the owner asked this part of the chat to follow, 2026-11-12): a model error in the chat, and the subscription usage limit that stands in for one (with the same red mark: cindy shows a usage limit as an error banner), is `ErrorNotice`: a `rounded-xl` card, 1 px `--line` border, `bg-surface`, 12 px padding; a 16 px `alertCircle` mark in `--red`, 2 px down; what happened in 13 / 20 `--ink` (it wraps); what helps in 12 / 20 `--ink-2`; then a wrapping row (8 px above and between) of the caller's buttons, 28 px tall (`Button size="sm"`: a height in `className` does not win over the button's own 32 px) with a 44 px touch target (`touch-hit`), followed by **Error details** / **Hide details** (a quiet button with a 12 px chevron, `aria-expanded`) when there is something to unfold; the details 8 px under it, the raw text in a `bg-inset` well with `rounded-control`, 8 px padding, 12 / 20 mono `--ink-2`, every character kept (`pre-wrap`, `break-all`) and scrolling past 160 px. No red fill and no red ring: red is only the state's mark, per 5.3 and "Color hierarchy and quiet surfaces" in `docs/web/web.md`. The status code and how many times in a row moved from the head into the details, so the head says only what happened and what helps. The card is drawn with 12 px padding, cindy's live banner's, also for an older error: cindy's in-stream record uses 8 px, but here one card shape serves both.

**The error the chat stopped on** (web, #5313; cindy's `ErrorBanner`, the owner's requested alignment, replacing the deviation #5157 recorded): while a chat's last line is a model error (the Persona conversation's last chat line, whatever feed lines follow), not dismissed, with nothing on its way and the Agent not working, it is drawn over the composer as the dock's last item: the same `ErrorNotice` with what can be done now (**Fix it**, **Retry**, **Edit last message**) and a ✕ (`onDismiss`): cindy's 28 px round button in the card's top end corner (`-mt-1 -mr-1`, so it sits on the first line of text), a 14 px x in `--ink-2`, the `hover` fill and `--ink` on hover, the accent focus ring, a 44 px touch target, spoken and hovered as **Dismiss error**. The chat leaves the line out meanwhile, so the error is said once. ✕ (remembered per conversation in this browser) and **Retry** (at once, before "continue" reaches the transcript) put the banner away; the same error again after a retry is a new banner. Dismissed, followed by anything, or while the Agent works, the error is the record in the chat: the same card without Retry, Edit last message or ✕, as cindy's `ErrorMessageCard` has none (a Persona chat's record keeps its Fix it: the hub's word on the turn's error while it is the last line, else what its own text names). A usage limit is the banner too, with ✕ and no Retry. The banner is `role="status"`; ✕ and Retry hand the keyboard back to the composer with a fine pointer only; a new banner is scrolled into view, once, in the box over the composer when that box scrolls; a Retry that did not go brings it back. The host's failure note (`FailureNote`, a botmux turn stopped on an error) and the Persona conversation's refused-send card (`FailureCard`) are the same notice, no longer red-tinted boxes: the reason with Retry and Open in Lark, or the hub's sentence with its buttons, the original under **Error details** and ✕; while the failure note shows, it is the one card over the composer and the model error stays in the chat. `ModelFallbackCard` is not a failure and keeps its look. Deviations from cindy: Esc does not dismiss the banner (Esc twice in the composer already edits the last message), and Retry's hover text says it continues the current task rather than that it resends the last message, which Persona's Retry does not do.

The hub's link while it is down (Waiting for network, Connecting…, Updating…, or what only the user can fix) is said once: in the title of a page that has one (an Agent's chat), else by a pill floating under the home bar on the home pages (iOS `ShellLinkPill`, `docs/apple/shell.md` "Connection status and toasts"; Android `JLinkPill`, #4339). It moves nothing, takes no touches and is announced; a spinner while it retries, a caution mark when it cannot.

### Web skill mentions (#5357)

Reuse the composer's existing `ComposerPanel` surface for `@` and `SlashPalette` surface for `/`, their neutral sliding
highlight, icon set, keyboard/IME/Escape behavior and context chips. A skill row has a readable name, source and
description; `@` uses two lines, while `/` puts them in its own 36 px row (44 px on touch). The source
distinguishes equal names. Selecting either row adds the same removable skill chip and opaque reference rather than hiding an invocation in display text.
Skill rows and the skill chip's remove target meet the Web 44 px touch size. The menu's loading, partial-inventory and
failure status stays secondary and never disables ordinary chat. No new primary screen, setting, model choice or
native client pattern is introduced. Use the existing surface/hover/ink/accent tokens in light/dark and narrow layouts.
Ordinary Claude Code and Codex Agent chats use this same picker and chip. The catalog belongs to the actual session;
unsupported or unavailable environments show the existing quiet failure status while text and other references remain usable.

### 3.11 Code blocks, inline code and quotes in replies

Drawn as Telegram draws them (#2984), with one look on iPhone / iPad, the web and Android; the frozen Mac keeps its hover Copy and plain quotes. Inline code lost its gray box on iPhone / iPad and the web (#9477); Android uses the same treatment (#9483). There was no component for these: replies used the Markdown renderer's defaults (a plain code box with Copy on hover, a thin bar), and the one code block with a title bar (`LarkCodeBlock`, Inbox) is the template, not a reply's.

| Piece | Look | Interaction | Source |
| --- | --- | --- | --- |
| Code block | a title bar on top (iOS `Theme.codeBar`) with the fence's language (none when it names none) and Copy (icon and word); the code under it in the monospaced font on the code well (iOS `Theme.codeBackground`, web `--inset`, Android `JCodeBlock`'s), scrolling sideways | Copy takes the whole block, with the "Copied" notice (and the success haptic on iOS and Android). Copy's hit area is the touch size (44 pt, past the bar's edges on iOS; web `touch-hit`, 44 px). A sideways drag on a wide block still goes to the code | iOS `ReplyCodeBlock` (`apple/Persona/Views/Chat/ReplyBlocks.swift`); web `CodeBar` (`web/src/ui/ReplyBlocks.tsx`), `CodeBlock` (`web/src/ui/Markdown.tsx`); Android `CodeBar` (`android/feature/chat/src/main/kotlin/persona/feature/chat/ReplyBlocks.kt`) in `JCodeBlock`'s `header` slot (`android/core/designsystem/src/main/kotlin/persona/core/designsystem/Lists.kt`), so every style's box holds the bar |
| Inline code | no background, border or padding (#9477): the system's monospaced face at 0.94 em (SF Mono's x-height is the body's; its wider letters made it look larger), CJK inside it in the body's system face (no font bundled; the web lists the body's CJK families before the generic `monospace`), in an ink a step quieter than the body (iOS `Theme.Reply.codeInk`, web `--ink` and `--ink-2` mixed 55:45, ≥ 4.5:1 on every page and card); words in an ink of their own (a quote, a quieter heading, an agent's quiet words) keep it, and in a written link it is the link's. About 0.12 em of room where it meets a CJK letter directly (iOS a thin space, `InlineCodeGap`; web a margin, `data-gap`), none beside a space or CJK punctuation. Android uses a thin space outside the code style and blends its primary and secondary inks 55:45 | a click or tap copies it ("Copied"); on the web so do Enter or Space when it is focused, and a pale wash shows under the pointer; a click ending a drag that selected words doesn't, and on Android neither does a tap in a message whose words are selected in place (4.2). Blue is kept for what opens: code that is a path there (inside a link) opens the file, not a copy. On iOS a long press still lifts the message | iOS `CodeCopyMarkdown`, `InlineCodeGap` (PersonaKit), `InlineCodeInk`, `RunFonts`; web `copyInlineCode`, `rehypeLinkedCode`; Android `copyingCode` (a `LinkAnnotation.Clickable` styled as code) |
| Quote | the accent bar on a faint accent card (iOS `Theme.quoteBackground`, web `--accent` at 8%, Android `accentSoft`), a quote mark at the top end (iOS `Theme.Icon.quoteMark`) | longer than about four lines (iOS `Theme.Size.quoteFold`, scaled with the text) it folds with its end faded; a click or tap opens it, its chevron (at the touch size) folds it again. A reply still being written never folds its quotes | iOS `ReplyQuote`, `replyStreaming`; web `ReplyQuote`, `ReplyStreaming`; Android `ReplyQuote`, `LocalReplyStreaming` |

The iOS in-place selection view (`LiftSelection.swift`) draws the quote card under the selection highlight. A code block's title bar is not drawn there yet.

### 3.12 Chat edges under floating chrome

Where a conversation scrolls under the chrome that floats over it, the content fades out under a fog; no line ever cuts a message, and the chrome's controls float over the fog (#2909, #3696). iPhone and iPad; the frozen Mac keeps its edges.

| Edge | Fog | Rules | Source |
| --- | --- | --- | --- |
| Top bar | page color at 82 % over material, solid under the bar, then out over 44 pt | the blur fades out over the first half of the fade, so text dims before it blurs; with the screenshot banner on (#9508) the solid part is the opaque page color under the bar (the banner under it must not show through), the fade below as before | `TopFog` (`apple/Persona/Views/Assistant/AssistantTopBar.swift`), `screenshotBanner` |
| Page footer actions (a page that isn't a chat) | material washed halfway back to the page's color (`Theme.groupedPage` behind a grouped list), whole from the bar's lower part through the bottom safe area, out along `EasedFade` to nothing a little above the bar's top | the page runs on behind the bar in a `safeAreaInset` and its last row clears it; the bar has no fill of its own, its pills (3.7) float over the blur. The material alone lit a black page's foot gray in dark. Not `safeAreaBar`: beside the iPad's floating sidebar it took 12 pt off a list's leading margin (its cards 8 pt from the sidebar, 20 from the other side), and on the to-do page its automatic edge was the hard one, a lighter band with a line along its top | `floatingFooter` (`apple/Persona/DesignSystem/FloatingPill.swift`) |
| The open Duo's tab bar along the foot (upright, Separate layout, #4231) | none of its own: the bar floats over the page's foot as the system's tab bar | each tab's controller keeps the bar's room at its foot (`additionalSafeAreaInsets.bottom`, as the system's tab bar), so the page runs on behind the bar, its last row and its composer clear it; none while the keyboard is up, when the bar is away | `TabSwitchLayout.bottomRoom` (`apple/Persona/DesignSystem/TabSwitchDock.swift`) |
| Footer (capsules, cards, composer) | page color only, clear `ChatView.newestGap` above the footer's top, solid from the capsules' bottom (`ChatView.underDockSolid`) to the list's bottom edge | in Persona's chats the list runs on behind the capsule row (`ChatView.underDock`) with the same extra room at its newest end, so messages rest where they would and the chat dims behind the capsules, gone by the composer; where the composer is the footer's top (Agents chats) it is gone by the composer's edge. The jump-to-latest arrow, the edge navigator and the footer are drawn over it | `FooterFog` (`apple/Persona/DesignSystem/FooterFog.swift`), `ChatView.underFooter` |

- Both fogs follow one eased curve (`EasedFade`: smoothstep in eight stops), so neither the start nor the end of a fade reads as an edge.
- At rest the newest message is clear of the footer's fog: the fog above the footer is exactly the room the list keeps at its newest end.
- A fog is a fixed-size layer at the list's edge. It never depends on the scroll offset, a streaming reply or keystrokes, so it is never drawn again while scrolling or typing (#3435); the keyboard only moves it.
- No solid band of page color over content, and no material under a chat's footer: on the solid page the page color alone shows what a mask would, without a tinted strip where nothing is behind it. A page that is not one solid color needs a mask instead.

### 3.13 Watch-face complications

Extension progress in `accessory_circular` uses a system circular capacity gauge. Its center shows the label on one
line, or the percentage for an empty label. Only this center text is capped at
`ExtensionSurfaceTokens.circularLabelLargestTypeSize` (`.xxxLarge`, matching `Theme.interfaceLargestTypeSize`) and
shrinks to `ExtensionSurfaceTokens.circularLabelMinimumScale` when needed to fit the fixed ring.
VoiceOver reads the label and percentage; finite ratios are clamped to 0–1 and nonfinite ratios become zero.
The circular gauge is independent of `progress.style`. `accessory_rectangular` uses a bar and `accessory_inline`
omits progress.

The watch widget extension doesn't build `Theme`, so complications take their fonts, marks and colors from `WatchFaceTokens` (`apple/Shared/Design/WatchFaceTokens.swift`, the documented fixed-size exception in 2.2), whose symbols mirror `Theme.Icon` with an `// icon:` note. A complication only draws what its app last wrote (a snapshot file in the watch App Group); all-day recording reloads only its own kind's timeline (`AllDaySnapshot.widgetKind`), not the other faces'.

- **All-Day Recording** (#4741, `AllDayRecordingComplication`; circular, corner, rectangular, inline, the Smart Stack): five faces, each in words and a mark, never by color alone. Off: "All-Day Recording" (the corner says "All-Day"), "Tap to start". Recording: "Recording all day" with the running time (`Text(_:style: .timer)`, so it needs no reload), in red as the system shows a microphone in use. Paused (by the user or a call): "Paused", "Tap to continue", `allDayPausedSymbol`, orange. Stopped by itself: "Stopped", "Tap to continue", the triangle, orange.
- **The stale face**: a recording snapshot is trusted only while the app keeps rewriting it (every 10 minutes while recording); the timeline carries a second entry at `AllDaySnapshot.staleAfter` (25 minutes after the last write), which shows "Check recording", how long since it was last heard, the triangle, orange. Why: if the app dies without a word, a face still saying "Recording all day" would be a lie.
- **Taps** open the app with the action the face offers (`persona://allday?action=start|continue|open`): start when off, continue when paused or stopped, the page when recording or stale. Only the app in front can open the microphone, so the face never acts on its own.
- VoiceOver reads the title, the running time as its value and what a tap does as its hint. `-openWatchItem allday-complication` previews every family in every state.
- **Record** (#5493, `MeetingRecordingComplication`; circular, corner, rectangular, inline, the Smart Stack): static, the recording mark in the system's red and **Record**, the word of every start ("One Record entry (#5243)"); the rectangular face adds "Meeting" under it (what a tap records; a longer line was cut short in en and ja). A tap opens the app straight into a meeting recording (`persona://record?meeting=1`), or marks the meeting while all-day recording runs. Its kind stays `persona.watch.meeting-recording`. `-openWatchItem record-complication` previews every family.

### 3.14 Home Screen and Lock Screen widgets

Extension widgets use `ExtensionSurfaceTokens`. In small, medium and large families, `progress.style == "ring"`
draws a fixed ring (`ringSize`, 50 pt; `ringStroke`, 7 pt) with a same-color track (`ringTrackOpacity`), a clockwise
arc from twelve o'clock (`ringStartAngle`) and round ends. Zero shows only the track; one fills the ring. Finite
ratios are clamped to 0–1 and nonfinite ratios become zero. The center shows a locale-formatted whole percentage
in `ringNumber` (15 pt rounded semibold at the default size), capped at `ringNumberLargestTypeSize` and shrinking
to `ringNumberMinimumScale`. The label sits beside the ring in the bar's `label` font on at most two lines, capped
at `canvasContentLargestTypeSize` and shrinking to `ringLabelMinimumScale` (0.5). Text that still does not fit uses
system truncation; detail is omitted. The row is one VoiceOver element with the progress label and percentage.
In a small widget, in a medium widget's two columns and in any Home Screen layout containing a ring, metric, bar label
and chart text is capped at `canvasContentLargestTypeSize` (`.xxxLarge`) and shrinks to `canvasContentMinimumScale`
(0.5), so a metric's value shrinks before it is cut short and the fixed widget canvas retains every line at
accessibility sizes (#9644). Other medium and large layouts keep their text unchanged.
`progressTint` follows the bar's default accent without tone coloring; the arc participates in system widget accents.
Items retain their order. A medium widget uses two columns only when its fitted view has exactly one section with
`[metric?, progress, chart]`, the progress a ring or a bar, and that chart has exactly one series and is not
candlestick: metric and progress on the left, chart on the right. Stacked, a metric, a bar and a chart are taller than
the medium canvas (#9644). All other layouts stack items vertically. An omitted, `bar` or unknown style
uses the default bar. Accessory families follow 3.13.

The widget extension doesn't build `Theme`: Persona's widgets take their colors, type, sizes and marks from `HomeWidgetTokens` (`apple/Shared/Design/HomeWidgetTokens.swift`), whose symbols mirror `Theme.Icon` with an `// icon:` note, as the watch's complications do. Heroes, rings and faces are fixed sizes (the system sets a widget's size); every line of words is a text style. A hero may shrink to fit (`heroMinScale`), and a name in a row shrinks a little (`nameMinScale`) before it is cut short. Every widget in `docs/apple/home-widgets.md`.

- **One widget, one job, one hero**: a number, a ring, a chart or a button, as the system's own widgets (Calendar, Reminders, Fitness, Batteries). A plain surface, no card inside a card; a small widget never holds a list.
- **One color per widget**, on its name at the top left and its hero; the agent states keep `StatusPalette`'s.
- **The persona is an accent**: a small face at the top right whose mood says how that widget's subject is, the empty states, and the state before pairing. It never takes the hero's place.
- **Every rendering mode**: in tinted, clear and Lock Screen modes every colored part goes in the accent group (`homeTone`, `homeAccent()`); a mark on a filled disc is knocked out of it; Mochi becomes a line drawing (`MochiGlyph`).
- **Counts** are the app's own (Needs You per session, section 4.8; the Inbox's Needs You; the Machines page's windows); inbox and health content is `privacySensitive`. A to-do ticked on a widget shows done at once and says so in red when the hub refuses it.
- **Choices live on the widget** (Edit Widget), never in the app's Settings: each starts at a default that works on a fresh install (every agent, every computer, the fullest account), and its list is the snapshot's, named as the widget names them.
- **A widget's own switch and buttons** (the Calendar widget's Today · Week · Month and its days, #6946): a segmented control drawn in `HomeWidgetTokens` (`segment…`: a track, the chosen part filled, fixed sizes like a hero), and days as buttons. Each is a widget button (an App Intent) that changes only that widget and redraws it at once, without the app or the network; Edit Widget keeps where it starts. Why a new pattern: a view people flip between often does not belong behind Edit Widget's sheet, and a widget has no other control.
- **The Lock Screen** shows the same hero in one color: the circle holds the hero alone, the rectangle the widget's line and one or two under it, the inline one line beside the clock.

### 3.15 Charts (iOS)

- `DataChart` (`apple/Persona/DesignSystem/DataChart.swift`): bars, lines and areas over categories, a few series in the data hues, holding a category reads it; a card's is compact (no axis text).
- `RingChart`: a donut (its total in the hole) or a pie, one series of a few parts.
- `CandleChart`: candlesticks with an optional volume chart under them; rising and falling colors come from the caller (the diff's green and red); holding a candle reads its prices, else the last one's.
- `TrendLine`: a trend line the size of a word, no axes, in the quiet ink unless given a color; VoiceOver reads where it went and its lowest and highest.
- `HeatGrid` (`apple/Persona/DesignSystem/HeatGrid.swift`, `Theme.Heat`): squares in the theme color's four steps from faint to full, no usage in the neutral fill (`Theme.Fill.chip`); the token calendar's days (a column a week, #9506) and its last week's hours (a row a day). The steps go by rank among the values above 0 (`HeatLevels`), never scaled from the largest, so one large value does not fade the rest. The squares keep their size at every text size and a wider screen shows more columns; labels stay few (three weekdays, the months). Today has a thin outline, a picked square a thicker one outside it; a tap picks, a second tap lets go, and the page says what the picked square holds in words above it. `HeatLegend` is its key (Less ▢▢▢▢▢ More, or 0 … the largest). The grid is one VoiceOver element whose value is the picked (or the last) square and whose adjustable action moves the pick.
- Charts never take a hex: series use role colors, the data hues or `Theme.Hue`; text around them stays in the text's colors.
- Machine monitoring has its own `MetricColor` palette on both Apple and Android: CPU blue, memory teal, download orange, upload purple and load magenta, with separate light/dark values from `spec/tokens/color.metric.json`. These series never reuse Agent state colors and do not change with interface style or theme accent. Android's additional disk series uses the registered neutral slate token. Holding a chart keeps one shared timestamp across the detail's charts; only **Back to now**, or that timestamp leaving the selected range, returns to current values. Missing series remain unknown and gaps never borrow a neighboring value.

### 3.15.1 Token usage calendar (Android)

`UsageCalendarCard` and `UsageCalendarPage` (`feature/assistant/UsageCalendar.kt`) are the daily-token counterpart of the Machine monitoring charts. They compare magnitude over time, so they use one hue rather than categorical series colors: the current theme accent in four quantile steps, with `sunken` for no usage. Text, dates, legends and breakdown rows stay in the ordinary ink tokens. The exact number and the four token kinds are available by selecting a day; the details page also lists every computer and kind in text, so color is never the only way to read a value.

On the detail page, each agent’s seven-day total adds input, output, cache write and cache read. Each positive total gets a grouped row with its Android agent mark, its name and a localized short number. The meter below the name compares the total to the largest agent on the same scale across rows. Historical usage gets a labeled row without a mark. At large text sizes the name can wrap above the meter. TalkBack reads the name and exact count together.

The daily grid is Monday-first, seven rows per week, with the current week last and a thin outline on today. A day with usage ranks among only the days with usage: `ceil(4 × rank / count)`, where rank includes every equal or smaller day, gives steps 1–4; no usage is step 0. The hourly grid uses the same quantile rule among its hours and retains repeated clock-change hours by timestamp. Small chart cells are not the sole controls: selected days have 48dp previous / next buttons and an exact spoken value, while the card's non-cell area opens the details page. Home reads while resumed and keeps the previous devices section until real data arrives; it never substitutes an all-zero calendar for a failed or unsupported read.

Month labels have their own visible header above the scrolling weeks. The header follows the scroll position and keeps only the latest complete labels that fit the width and text scale; it never clips a month to a partial word. Weekday labels keep their full localized glyphs inside a dedicated column, while the day cells themselves stay the same size at every text size.

### 3.14 Tabletop hinge safety (Android)

A horizontal separating hinge divides a page into safe regions; the window and hinge alone select this behavior, never a device model. All three Android styles use the same measured policy in `PageFrame`.

- With a bottom dock, content stays above the hinge and the dock starts below it. The dock retains the page's reading width and does not add a second navigation-bar inset.
- Without a bottom dock, every interactive and focusable element belongs to the upper safe region. The lower region and the hinge contain only the page background.
- The upper region must fit the measured top bar plus Android's minimum touch-target height (`Tokens.MinTouch.android`) of usable content. The lower region must fit the dock's measured height when one exists. Otherwise the page uses its ordinary flat layout and system insets rather than clipping two half-pages. A dockless page may use a short lower region as background.
- Content ends `Tokens.Space.m` before the hinge. Bars are measured before the policy runs in the same layout pass, so there is no first frame across the hinge followed by an `onSizeChanged` correction.
- Split and flat layouts reuse the same content slot. Rotation, folding and window resizing do not reset navigation, list position, drafts, selection, focus or ViewModels.

Real folding-feature evidence and fake-posture layout evidence are reported separately. `debug.posture=tabletop` does not prove an OEM device's posture behavior. Glass requires light and dark captures; Brief and Board require light and dark samples. These rules change geometry and insets only, not the styles' visual languages.

## 4. Interaction patterns

### 4.1 Navigation hierarchy (super rule 3)

`docs/SUPER_RULES.md` rule 3: Assistant and Agents are the only primary home destinations on iOS, Android and web. Settings opens from the sidebar. Everything else (Recordings, Inbox, To-dos, Scheduled Tasks, Knowledge, Machines, Health, extension pages) is pushed from the drawer. Push by value for new pages; mixing view-based and value-based pushes in one stack is a known trap. An optional Messages page can be switched on and becomes a third primary destination; whether that fits rule 3 needs the rule owner (DS-26).

On Android, a Merged sidebar pick replaces the visible root and closes the compact drawer or regular overlay.
A beside column stays open. Navigation inside the visible root keeps its Back path, and external entries retain their
route and session identities. The sidebar setting changes the presentation of Assistant and Agents.

A dispatched task's Open link (#3711) is a compact row using existing typography, fill, radius and 44 pt touch-target
 tokens, not a filter or a link to the whole In Progress list. One task shows its title and Open; a plan lists only its
 own tasks in order. On iPhone it pushes the existing Agent page by `SessionKey` on the Assistant stack, preserving
 Back to the conversation; on Mac it opens the task's detail. Before an Agent is available it opens that task's
 starting/failure detail. This targeted navigation correction reuses existing pages; no new visual style or setting.
 Details and wire contract: `docs/assistant/unified-entry.md`, "What a dispatch opens".

### 4.1.1 Apple settings scope and search

The shared Apple Settings page follows three scopes, in this order: **Current Assistant's Settings**, **This Device**,
and **Hub**. With several installed assistants its first heading reuses `AssistantSwitcherHeader` (avatar, name, state,
and a one-line Switch label); with one it retains the chosen Persona face and name, without a roster switcher.
A caption explains that only the current assistant's settings are shown. Device and Hub captions explain their shared scope.

Common model choices open **Models**, with providers and keys under **Advanced**. Runtime configuration and session triage
open through **Agent**. Appearance and display have a device-owned page; the shared avatar belongs to Hub. Diagnostics
(build commit and the last new-chat check) are folded inside About. Watch sync and disconnection belong to Devices & Pairing.
Existing settings route values remain valid; changing assistants returns the settings navigation to its list and retires old drafts.

Search reuses the system searchable field and SettingsRow navigation. It searches localized entry names and common aliases,
including developer terms, and opens the existing destination directly. Results have stable catalog identities; a query changes
only navigation, never a setting. Pairing and storage use the `Theme.Icon.pairing` and `Theme.Icon.storage` vocabulary tokens.
Why this arrangement: the earlier unscoped list mixed shared configuration with device preferences and exposed expert controls
before common choices. It introduces no visual style, color palette, model default, or primary navigation destination.

### 4.2 Gestures

| Gesture | Behavior |
| --- | --- |
| Right drag on Assistant root | opens the drawer, unless a pushed page, a sideways scroller or typed text is under the finger |
| Horizontal drag between pages | pager with 3x resistance past the ends; flick above 300 pt/s or past half way |
| Edge swipe back | system on a pushed page (the pan yields to it). At a root, Android excludes a left-edge strip so the drawer or column opens there instead (`RootEdgeExclusion`, #9803); the right edge stays with Back |
| Swipe back from anywhere (iOS 26) | the system's, on a pushed page that shows the system's Back: under iPhone Duo's side bars the Agents chat and a task's page use the system's Back for it (#9971). A page with its own header (the system's bar hidden) has the edge swipe only: UIKit gives the swipe from anywhere no touch there. A pane's own rightward swipe (files / Git back to the chat) comes first |
| Swipe on a message (Agents) | quote, threshold 66 pt, with a VoiceOver "Quote" action |
| Drag on a conversation | puts the keyboard away as soon as the finger starts to drag, not at the touch, which may still be a tap; a field in a request card loses it too (#4210). iOS: `ChatView`'s `.interacting` scroll phase calls `Keyboard.dismiss()`; Android: `ChatListEffects` on the list's `DragInteraction.Start`, for the Agent chat, Persona's conversation and Lark chats, in every style. A tap on blank space does it as well; a scroll the app starts (following the newest message, a jump) does not; Android's keyboard does not follow the finger |
| Long press on a message | your own folded long message opens in place, with no bar (#5676); otherwise a lifted menu with an action bar, over the whole page dimmed (a page's floating top bar included); a message taller than the room shows the part pressed and scrolls inside, the bar fixed; a selection handle held within `Theme.Lift.edgeScroll` of its top or bottom edge scrolls it on, up to `Theme.Lift.edgeScrollSpeed`; its selected words take touches `Theme.Lift.handleReach` past them, so a handle's knob over the first line or under the last can be dragged (#5291); the edit menu over selected words goes above them, or below when there is no room, never over the words or either handle (`Theme.Lift.handleReach` of room for each handle's knob, #6005); a message only on this device ends the bar in a red Delete (Don't Send while it waits), Undo in a toast (#4626); a sent message the chat can pick ends the bar in a row of its own under a hairline, Select Messages (#9468), as wide as the actions, which stay as they were |
| Swipe a To-dos row (iOS, hubs with `todo_view`, from #10465's switch-on) | one trailing swipe from the end, as the Inbox's: Done (the full swipe) and Someday; Reopen on a closed to-do, Undo on one ticked a moment ago, nothing on one still being saved, Try Again and Remove on one nobody knows the hub has. Long press: Today, Date…, Project, Someday, Copy Link, Delete… (Undo or Reopen first where the swipe has it). VoiceOver has the swipe's actions and Delete… on the row, and the tick is its own button (3.4 "To-dos list") |
| Swipe an Inbox row | one trailing swipe from the end: Done, or Move Back on a done item (the Done section's rows); none towards the start, none while picking items or while the row's write is out. A full swipe on iOS; on Android the swipe component's threshold. VoiceOver and TalkBack have the same action on the row (#4210) |
| Select Text on a message (Select on Android) | selects the message's words where the message is, the system's way: handles to drag and the system's edit menu, which has Copy and, where the chat quotes, Quote (not for a message only on this device). iOS (`LiftSelection.swift`, #603, #2962): the lifted copy of the message draws its words again in a text view laid out like them, because MarkdownUI cannot select; Select Text selects all of them, a second long press the word under the finger; the bar steps aside for the edit menu, Copy and Quote close the lift, and a tap on the dimmed chat or one that clears the selection brings the bar back. Android (#4210 T5, `MessageSelection.kt`): nothing is lifted. The message's words are put in a `SelectionContainer` where they are and all selected (`SelectionState.selectAll()`); a code block, a list and a quote are among them, a code block's title bar and a card are not. A long press on a word of the selected message picks that word, as iOS's second long press does (Compose has no public way to start with the word under the finger; Select takes the whole message, as iOS's Select Text does). Copy puts the words on the clipboard as the chat shows them (no thin spaces round inline code, a list item's marker on its line); Quote adds them over the composer as a quote of the picked words (`part="selection"`, "Quoting selected text · name"). The message has no long press, swipe to quote or inline-code copy while it lasts. It ends when nothing is selected any more (a tap on the words, Copy, Quote), when a tap starts outside the message (a drag is a scroll and does not; the tap goes on to what it hit), on Back, and when the message has gone from the screen; a press on another message ends it too. Select no longer opens a full-screen reader; the reader stays where else it opens (a long message's "View full text", a code block, a thought, a plan, a tool's raw input and output) |
| Tap or hold a mic (iOS: the composer's, the BeautifulUI style's and Health's composer, a to-do's notes, #9619; the meeting page's Talk, #9658; Android chat and to-do notes, #9657) | a tap starts dictation and a second tap in the same place stops it, the words kept and nothing sent (the composer's tap opens the dictation bar, ■ where the mic was, section 3.1). Touched and held for `Theme.Hold.delay` (0.3 s) it dictates only while held: letting go keeps the words, sliding left 150 pt (100 pt, or a 400 pt/s flick, as the finger lifts) drops them; the composer shows the strip with the red dot, the time and "‹ Slide to cancel", the other mics their listening face. One touch is a tap or a hold, never both: a tap never starts a hold, letting go of a hold is no tap (`MicPress`). No slide up to lock: the tap is the hands-free way. A touch the system takes away, or the mic leaving the screen under the finger, keeps what was heard. VoiceOver's activation is the tap. Talk differs where it must: its span opens as the finger lands (a tap keeps it open), and it has no slide to cancel (section "Meeting page", Talk). Android measures slides in dp and flicks in dp/s. The phone’s voice-input activity handles taps on devices that only have that activity. |
| Touch and hold the composer's + (iPhone) | system menu of the attach panel's More list; a tap still opens the panel, whose More tab is the plain alternative (#3225) |
| Tap a photo in the + panel's grid | its ring (and a 45 pt corner round it) picks it, as before; the rest of the photo opens it in the image editor. VoiceOver's activation picks, Edit is an action (#4963) |
| Drag in the image editor | Box and Pen draw (Box is the default tool); in Crop, a corner or edge within 22 pt resizes the crop, inside moves it. Crop's shapes, Rotate and Reset are the controls that need no drag (#4963). The web's the same in CSS pixels, a mouse's cursor showing which handle it would take (#4991) |
| Link card drag | to 0.6 it stops blocking the chat; further down it closes |
| Terminal cover drag | its bar follows the finger; past 140 pt or a fast flick it closes. The bar, with a 44 pt Close, is there in every state; with no terminal the whole area drags (#4328). Android the same (140 dp, a flick whose half second of glide passes 420 dp; `TerminalCover`, #4339 C7), its bar under a handle, with Back closing it too |
| Tap on a saved place's map | moves the place there (#4248); search and Use Current Location do the same without the gesture |
| Drag the sidebar (iPad, open Duo; #4206) | at its one width, the line between the column and the page follows the finger: from the window's leading edge (a finger's width in) while the column is away, from the line beside it (a tap target wide) while it shows. Pictures of the screen taken as the finger lands, the top and side bars in them, are stretched or squeezed with it, both blurred (most at 80 pt), with a 3 pt guide line in the theme color on the line; where the column lies over the page the page's picture stays and darkens as the column comes out. Let go, a flick above 500 pt/s or past halfway decides out or away, else it snaps back; the line springs there (with Reduce Motion it is there at once) and the pictures fade. Where the page can go back, its swipe back keeps the edge. The rows' own swipes and the list's scroll start only once the drag has failed (it fails at once off the line or moving up and down). The sidebar button is the plain alternative. Where it is left is remembered with the column's shown state. The column is never resized |
| Hold a to-do on the calendar | after 0.35 s held still it follows the finger (a pointer: once it moves) to another time, day or All Day row; saved on release with Undo in the toast; "Change Due Date…" for VoiceOver (`docs/assistant/todos.md`, "Calendar") |
| Long press / trailing swipe on an extension's row or tile | its extra actions (`.extensionRowActions`, UI version 6): the menu has them all, usual ones first and destructive ones red and last; the swipe has those marked `swipe`, the destructive one at the edge, never run by a full swipe; one with `confirm` asks in an alert with Cancel; VoiceOver has them as the row's actions |

Every gesture-only action needs a plain alternative for VoiceOver and Switch Control (see the audit).

The web user-input navigator (#4836) initially exposes the latest 50 genuine inputs, or all inputs in a shorter
conversation. Upward transcript loading extends that range when it reaches earlier inputs; global ordinal labels
still count from the full history. Reuse the existing rail, hover preview, keyboard controls and mark geometry.
This requested pagination correction does not add a setting or change placement. Android has no corresponding
navigator on current main and is outside this fix's scope; it must not use its two-turn body window as a global count.
When a web selection needs older messages, keep its loading status at the rail while the current transcript stays in
place. Only the latest selection can move the transcript. If it fails, keep the marks available so it can be selected again.

The web Changes panel's secondary file tree (#10477) reuses the existing `ResizeHandle` separator from the sidebar and side panel: pointer drag, arrow keys, Home/End, and double-click/Enter to reset. Column dimensions live in `web/src/lib/layout.ts` (`CHANGES_TREE_DEFAULT`, `CHANGES_TREE_MIN`, `CHANGES_TREE_MAX`, `CHANGES_DIFF_MIN`). Its default remains 192 px; the browser remembers a user resize and temporarily fits it to the panel while keeping space for the diff. At narrow widths the tree stays above the diff. File names and indentation may truncate, while each row's unchanged addition/deletion counts remain whole and the full path remains on hover.

### 4.3 Feedback and haptics

Message feedback (#5258, principle 7, super rule 5) reuses the chat's status line and pending bubble, not a new card or
style. Show Sending immediately from local state, then Processing on the hub's `message_send_state` receipt; do not wait
for a model token or overwrite `Session.status`. The pending-send overlay is scoped to the conversation and message,
survives stale snapshots, and hands off to actual execution or queued state on delivery. Failure removes only its own
overlay and keeps the existing draft, failed-bubble and uncertain-delivery recovery. On the web the state is said once,
by the status line under the last message (Sending, Processing, delivery unknown); the pending bubble only dims, with no
second line under it (a queued message's bubble keeps its Queued). The full wire and reconnect rules
are in `docs/protocol/protocol.md` ("Message feedback (#5258)"). Apply the same definition in Assistant and Agent chats.

| Message state | en | zh-Hans | ja |
| --- | --- | --- | --- |
| Local input pending | Sending | 发送中 | 送信中 |
| Hub received; execution may still be preparing | Processing | 处理中 | 処理中 |

These are message states, not answers to an approval or a claim that the model is already working. Receipt alone does
not enable Stop or promise cancellation of the pending HTTP request. Keep existing answer
labels and the authoritative Working/Needs you/queued state words. Verify dark/light in the existing styles and measure
send-to-feedback separately from send-to-input delivery; a transport receipt's queue timestamp is not a client paint.

`Haptics` (`apple/Persona/App/AppModel.swift`): `tap` (light impact), `selection` (a tick), `success`, `error`, `attention` (warning). 443 call sites on 11-11 (tap 243, success 129, error 57, attention 14); no-op on visionOS; generators are created per call. One direct UIKit use remains (`apple/Persona/Views/Chat/ChatEdgeScrub.swift`). Android has the same four kinds (`android/core/designsystem/src/main/kotlin/persona/core/designsystem/Haptics.kt`: tap is a context click, success confirm, attention a gesture threshold, error reject; the system's touch-feedback setting decides whether they play).

Which action gives which haptic, on both (#4209; the rule iOS follows):

| Action | Haptic |
| --- | --- |
| Copy | success, with a "Copied" notice (Android: every copy goes through `rememberCopy()`) |
| Send, Stop, switching pages, opening the avatar menu | tap |
| Carrying something to a new place: a to-do held on the calendar (#2045) | tap on the pick-up; selection at each new quarter hour or day it would land on |
| Pressing Approve, Deny or a choice | tap on the press; success when the answer went through |
| Archive a conversation or an Agent | tap, then its "Archived" notice (iOS `archiveConversation`; marking a to-do done on today's To-dos list plays none) |
| Done or Move Back on one Inbox item, by swipe, menu or the Lark chat's button | tap, both ways (iOS `setDone(_ item:)`); many at once (Mark all as seen, Clear all, the picked rows' Mark done) buzz success when filed and tap when put back (iOS `run`). Android's conversation rows count as one item (#4210) |
| A to-do ticked, put in Someday, deleted or added on the To-dos list, and its Undo (iOS, hubs with `todo_view`, from #10465's switch-on) | tap, as Done on an Inbox item; error when the hub turns the write down |
| Save, an answer or approval that went through | success |
| A request newly arrives, or what the user acted on changed and must wait | attention |
| An operation failed | error, paired with its error notice |

Watch maps success, failure, notification, stop and retry the same way. A watch haptic plays only while the app is in front (watchOS ignores one from the background), so whatever must reach a wrist that is down goes as a notification too (all-day recording's pauses, 4.5). Web has none. Android follows the copy row everywhere since 11-11, and the press rows in its shared components: Send and Stop (`SendButton`), every answer (`DecisionButtons`, `DecisionAnswers`, `DecisionActions`, `DecisionOption`, `DraftDecisionButtons`), the page switcher (`HomeNavigation`, only when the page changes, as on iOS) and the persona button (`JPersonaButton`); a request that arrives plays attention while the app is in front (`InteractionCenter.arrivals`: a card the hub pushes that none showed yet, not one a snapshot lists). The other rows are adopted screen by screen (DS-23).

### 4.4 Confirmation and undo

Severity ladder to adopt:

1. Reversible: do it and offer Undo in a toast (inbox bulk actions, remove from list, folder trash do this).
2. Destructive with one object: `.accessibleConfirmationDialog` on Apple (rule 7 in section 1.2), with a destructive button that repeats the verb, a message that says what is lost, and Cancel.
3. Needs extra choices (keep or remove the worktree): a content-height sheet.
4. `.alert` for short prompts and text entry, except where the text needs what the alert would cover (below).

A write the hub cannot undo may still offer Undo by holding it on the device (iOS To-dos list, hubs with `todo_view`,
from #10465's switch-on): Delete… on a to-do confirms (rung 2), then its row leaves with "Deleted · Undo" while the
DELETE waits out `Theme.Motion.todoUndo` on the device, so Undo sends nothing; a tick and Someday wait the same way.

In Android Agent Files (#11733), **Move…** uses a folder picker sheet with an explicit **Move Here** action. The conflict
dialog opens for a name the hub reports as taken and offers **Keep Both**, destructive **Replace**, and **Cancel**.
Keep Both gives the moved item a numbered name. Replace puts the existing file in the computer's Trash.

In-meeting typed questions (#4486) are the exception to rung 4: they use an inline input row in the live recording sheet's bottom bar (`apple/Persona/Recording/RecordingLiveBar.swift`), not a system text-entry alert. The question usually refers to what was just said, and a modal alert hid the live lines and the earlier answers while the user typed. It also allowed only one line and no reference back. The keyboard button beside Ask Persona opens the row (at accessibility sizes, More › Type a Question opens it). The row shows a hint, then a field and send button that reuse the composer's `ComposerSurface` and `ComposerRoundButton` (3.1); the two are one view, `MeetingQuestionField` (`apple/Persona/Recording/MeetingQuestionField.swift`), which the meeting page's composer reuses ("Meeting page (#3862)" below), so the two stay alike. The field grows to `Theme.Recording.askLines` lines (`askLinesLargeType` at accessibility sizes, where the hint also steps aside while the keyboard is up and the field's spoken hint carries it). Send stays in place but is dimmed and disabled until there is non-whitespace text, and Return sends too. While the row is open, the keyboard button becomes Cancel, which drops the draft as the alert's Cancel did. While the field has the keyboard, Pause, Stop and Bookmark step aside so the timeline keeps the room. Sending goes through the same `MeetingAsk.ask(_:text:)`, then clears the field and closes the row. It is not a fifth composer (DS-19): it has no attachments, dictation (the microphone is recording), model menu or full-page editor.

**Send now** on the web's queued messages acts at one press, with no confirmation (#7155; it used to open a `Dialog`, and an in-place second press was tried and dropped), and so does Send Now on the iOS queue strip (#7169; it used to open a `confirmationDialog`). On this ladder it is not destructive: nothing is lost, since the messages go now rather than at the Agent's next step, and only the step under way is cut short. Next to it, Delete (which does lose the message) and the composer's Stop and Esc (which cut the whole turn) act at one press, and Claude Code's own send-now key asks nothing, so a confirmation here was the odd one out. What the dialog said (what the step under way does) is the button's tooltip and its accessible description on the web, and its VoiceOver hint on iOS; the toast that follows says they went (on the web, to whom).

The web Agent close action uses `capabilities.close`, with terminal-only closing retained for older nodes. A runtime that can close a conversation uses the existing close confirmation even when it has no terminal controls. The confirmation checks the current row before sending, so an already offline session simply dismisses it.

An Agent has one close action in the web's menus, **Close…** (#7125), where **Close agent…** and **Archive and close…** used to sit side by side. It is rung 2: not busy, it archives and closes, the button says **Archive and close**, and the toast offers Undo. A busy Agent's Close… is not greyed out: the confirmation starts with one line in `--red` naming what closing interrupts (an answer it waits for, its work, a loop, a server), and the destructive button reads **Close anyway**. **Archive** stays separate and waits while the Agent is busy, saying why, because it only hides the Agent. A pane with an Agent closes through the same confirmation, so no second path closes a working Agent without the warning; Close pane sits with the pane's own actions (New pane, Git), not in a danger group of its own.

Clearing the main chat's context (**Clear context** in its ⋯ menu and `/clear`, #5095; the web, Android, and iOS — on iOS the avatar menu, the sidebar main-chat long press, and `/clear`, #5100) is on the reversible rung without an Undo toast, a user-requested alignment with Cindy's `/clear`: it runs at once from the menu or the command, and its only feedback is the chat emptying in place (the same top bar, an empty middle, the composer still at the bottom; no jump to the centered new-chat page). Nothing is deleted: the old chat is archived as "Old main chat · until <date>" and comes back when it is sent to. Failure is a toast with the reason, and the `/clear` goes back into the box. **New main chat…** keeps its sheet, since it has a choice to make (carry a summary).

Deleting a Persona chat (#6172, web first) serves everyday people and reuses Menu, Dialog and Button without new tokens. **Delete…** appears in sidebar and History menus. The destructive one-object dialog has **Delete chat** and Cancel, with copy stating that Persona history is removed, task and Agent references and previously saved knowledge remain, and the computer transcript is retained. No physical-file checkbox is offered. It appears for ordinary and archived former main chats when the hub supports it. The main chat uses the existing Clear context action before its archived predecessor can be deleted. Cancel and repeat submission are disabled while the request is pending. Busy or waiting work is refused inline without removing the row; success removes it, while an already open reference shows **Conversation deleted** without a composer.

Removing a link between two Knowledge entries serves everyday people. The web dialog (`Relationships.tsx`, #10333) and the
content-height sheet on iPhone and iPad (`KBRemoveLinkSheet`, #10481, rung 3) name both entries and say what stays out.
**Also stop automatic changes to both lists** starts off each time. Turning on the `Switch` or `Toggle` names both lists
it will protect. Apple's sheet keeps **Cancel** and the destructive **Remove Link** visible in a vertical stack of themed
buttons while the explanation scrolls when it needs more room. The link disappears immediately on submit, then the app checks both entries.
**Removed by you** follows the related entries and offers a secondary **Allow again** action on each row. It makes the
narrow removal reversible by letting Persona consider the link in a future update.

A blocked terminal card on the web offers **Close Agent**, including when no choices can be parsed from the screen.
Its footer reuses `Button`, explains that the pending confirmation is cancelled and the conversation kept, and
requires **Close and archive** or **Cancel**. Close controls use the existing 44 px grid minimum and single-line labels.
The archive toast offers Undo; Undo restores visibility without reopening the Agent or granting its request.

Task-progress reply drafts (#3743) use the reversible rung: **Send** is primary, **Later** and **Don't handle** are
secondary. Don't handle removes only the exact draft from Needs you, sends nothing and leaves its task, Agent and
Inbox source open. It persists a task decision and offers receipt-scoped Undo in the web's existing six-second toast
or Android's toast (`JToastHost`, which draws the button in the accent, as iOS). A newer progress draft
remains eligible; Undo cannot overwrite it. Web uses `Button` with
44 px grid-based minimum targets and stacks the action group at narrow widths. Android reuses `DraftDecisionButtons`
with the existing 48 dp button token and stacks the entire group at large text sizes or narrow widths, primary first.
A message card in Persona's conversation on iOS and Android's Glass style adds **Edit** beside Send: the drafted reply
reads as text until Edit opens it for typing, and **Done** closes it (iOS `MessageCard`, Android `DraftDecisionButtons(edit)`).
The original card's not-sent receipt remains inspectable. This deliberately does not reuse Inbox completion, which
would suppress future progress replies for the same source. The API contract is in `docs/assistant/agent-work.md`.

A hand-off write-back card (#5932, web `HandoffCard`, iOS `HandoffCardView`) is the same card for an Inbox item the
user handed to the conversation: the item's summary over the reply, **Send** primary (**Got It** for a conclusion),
**Edit** on iOS beside it (the web edits in place) and **Later**, which folds it to one line. It has no Don't handle:
the item is still in the Inbox, where it can be turned down. Its state comes from the item, so it is never a second
count of the same request. The main conversation's **Wrap up: <title>** chips over the composer follow `RouteChip`'s
capsule, one per open item handed to it, at most two.

Today there are seven mechanisms and the same action can use different ones (DS-24). Archive in the sidebar, to-do swipe delete and mail-account delete have no confirmation and no Undo. Only the Mac discards edits with a prompt.

Saved Agent searches (#7753) reuse the command palette and one replacement `Dialog`, switching between its list and
editor instead of stacking modals. Empty-query shortcuts follow recent chats; applying one fills the search field
without executing an Agent action. A version conflict keeps the editor draft and shows the current saved values;
replacing them requires explicit review. Delete is reversible through a revision-guarded Undo, while ordering uses
visible up/down controls as a keyboard alternative. Returning restores the prior query and focus. These secondary
controls and their code load on intent, not as another primary destination.

The web command palette's runtime-Agent search (#7624) adds a status line and More/retry controls outside the
listbox. Local results appear immediately; bounded server pages supply authoritative matches and evidence. New
results preserve the selected identity rather than the previous numeric row position; a removed selection requires
a new keyboard choice before Enter opens anything. Loading, failed, expired and loaded-only fallback scopes are
stated explicitly. This reuses the existing palette, controls and localization without a new search destination.

The optional web workflow board's manual view supports bulk metadata edits (#7583). **Select** reveals native
checkboxes and a selection bar using the existing `Button` and `Menu` components. **Select all visible** counts only
cards in the current filters and, on a narrow board, the displayed column; hidden cards are not silently included.
The bar wraps whole controls without wrapping button text. Movement, pinning and archive/restore use the reversible
rung: immediate feedback, one atomic write, and **Undo last change** while its acknowledged board revision is current.
A conflict reloads without replaying the operation or its inverse. This pattern belongs to the optional Agents board,
not Assistant's primary chat. Older managers and view-only connections retain the existing single-card experience.

The optional workflow board's **Activity** view (#7599) is a read-only `Dialog`, with a matching expandable section
inside workspace details. It fetches compact revision summaries only after opening and paged before/after changes
only after expanding. A new-activity hint preserves the reading position until the user requests the latest page.
Historical names remain as recorded; current workspace navigation appears only for an existing card. An update that
involves several cards stays one group, including when reached through a card's history. This uses the same buttons,
loading/retry states and timestamps as other secondary views, and adds no primary destination or rollback control.

### 4.5 Push, notifications and reminders

- The system prompt appears right after pairing and at launch, with no in-app explanation first (`apple/Persona/App/PushRegistrar.swift:41-53`, the options at `:174`). Requested options include time-sensitive. When denied, Settings explains and deep-links; allowing it there registers when the app comes back (#1861). A confirmed outage also puts one non-dismissible `NoticeBanner` on Persona's main screen (#9612): plain words for what cannot reach this device and one repair action, with denied permission deep-linking to system Settings and other causes opening Notifications settings. Unknown or unchecked hubs, a healthy route and a route whose only problem is one failed push stay silent; raw relay, APNs, HTTP and credential details remain in Settings.
- Foreground: the system banner, suppressed for the visible session. No custom in-app push banner.
- Actions: Allow (needs unlock) and Deny on permissions; Approve and Send Back on plans; Reply on done (`apple/Persona/App/NotificationActions.swift:19-52`). Failures are posted as local notifications.
- Live Activity: Allow and Deny buttons; Allow requires authentication. Stale after 15 minutes.
- Recording Live Activity, a meeting app's (#8731 V4): the name with a still waveform (green while it records, gray paused) and the time, Persona's part and "Topic 2 / 4: <topic>", the latest line in quotes or Persona's answer in a purple box. Then Bookmark · Ask Persona (while Persona answers) · End, each with its words. A meeting's End opens the meeting and asks "End the Meeting?"; any other recording's Stop stops it at once, without unlocking (`docs/apple/live-activity.md`, "Recording"). In a room meeting with Talk (#3862 phase 4) **Talk** leads instead: its tap opens a talk window (Live Activity buttons can't be held), the button reads **Done** while it is open, the third line says "Listening to you…" and the island's mark is the talk mark.
- Apple Watch all-day recording (#4741, `WatchAllDayNotices`, `AllDayAlertPolicy`): the watch posts its own local notifications, since the phone may be out of reach and only the watch knows its microphone stopped. "Recording Paused" (a call, Siri, a changed microphone, a failed resume) and "Recording Stopped" (battery, storage, a failed write, an audio reset, the app ended) each say what happened and the one next step (a stop also that what was recorded is kept), with **Continue**; the phone's request to start has **Start**; a deletion on the phone has none. Each kind has one request id, so a newer one replaces the older. In front a notice goes only to Notification Center (the page and a haptic already said it), and recording again withdraws it. A "Recording Stopped" scheduled 10 minutes ahead and pushed on by every saved chunk is the dead-man's switch: it fires only if the app dies. A tap on the notice opens the page; its button brings the app to the front, where the microphone can open.
- Deep links: the `persona://` custom scheme, plus the former name's scheme kept for old links (`docs/protocol/rolling-upgrade-compatibility.md`); no associated domains.
- Reminders are agent tools and Scheduled Tasks, not a UI object.
- Wording differs per surface for the same event (section 5.4).

### 4.6 In-app links card

Links open inside the app in a card that fills the screen. Dragged down to 0.6 it stops blocking the chat. Further down it closes. Speed matters first. Implemented by `apple/Persona/Views/Link/LinkSheet.swift:111`. Header buttons need 44 pt frames (see the audit).

A computer's dev server opened from a chat (a forwarded port, `docs/web/preview.md` "iPhone") keeps the same card and adds a bar under the page: back, forward, the page's address on that computer, a screenshot (Visible Area or Whole Page, into the message box) and Pick an Element. The bar's buttons are `ThemedIcon`s in 44 pt frames (`tapTargetFrame`), the address a `Fill.well` capsule. Why a bar and not header buttons: the header already holds ✕ (first, #9653), Refresh and ⋯, and these are the page's own controls, where Safari puts them. Picking is a mode, not a gesture: a capsule over the page says what a tap does and has Cancel; a tap picks without pressing, the element is outlined in the accent color, and the bar gives way to the line to write about it (Cancel, Add to Message), which becomes an element comment card over the composer. The servers list (`PreviewChooser`) is an inset grouped list in a sheet; Start… asks in a content-height confirmation sheet like Close Agent.

### 4.7 Tool execution collapsed (super rule 2)

Tool execution is one summary line, never opens by itself, shows the latest text segment while running and the result when done, and expands by hand. Opened by hand, a step (and the summary itself) changes in place: the tapped row stays under the finger, what opens fades in, what closes goes at once, and the list holds still while the turn goes on (new steps, the answer, the next turn, the footer under the list changing) until the user scrolls, taps ↓ or sends (`toggleInPlace`, `ChatFollow.toggledRow`, #3005). iOS complies: `isOpen` ignores `live` (`apple/Persona/Views/Chat/ChatLines.swift:293`) and tool rows use `autoExpand: false`. The "latest segment only" half was not confirmed from code reading and needs a screenshot test.

**The opened summary** (web, #10247) is a flat sequence of narration and action groups, sharing one rail and indentation. The outer summary keeps the whole-exchange clock and collapsed default. Action headers name at most three kinds in first-seen order, with the first kind's icon; read/edit/search/command kinds have localized counts, and other kinds retain their existing labels. Earlier live groups start collapsed, the current one opens within the trace the reader manually opened, and a superseded run closes; all groups can be opened and closed by hand. Newly loaded completed groups start open; remembered disclosure choices remain available. Latest narration stays only in its text slot until superseded, and the final reply appears once. Rows lead with the file, query and target, or command; command descriptions remain in tooltips and expanded detail. Reliable per-call durations appear only from 30 seconds; the total exchange clock stays on the outer summary and run bar. The current run previews edit diffs using the existing diff renderer. The flatter layout replaces repeated time headings and nested rails so the inspected work is scannable, while preserving the top-level progressive disclosure required by super rule 2.

A long finished summary draws its first 60 steps with the tap and adds idle batches of 60. Live summaries, search hits and jumps draw all requested steps immediately. Search, jump-to-step and folded history continue to open the groups holding their target (`docs/web/web.md`, "A long summary opens a screenful at a time"). A thought with words is one row in the tool rows' chrome (16 px status slot
with the sparkle, italic `thinking-ink` words on one line, the 18 px chevron slot), opening in place only when it does
not fit or has a line break, its opened words selectable; a thought without words is no step, so it neither makes a
summary of its own nor splits a run of rows. The live "Thinking" line with dots and a clock ends
the innermost live summary, once. Durations read `6s`, `1m 5s`, `2m` in every language and, from an hour, the
language's form that always keeps the minutes (`1h 0m`, `1 小时 0 分`, `1時間0分`). Copy follows cindy's catalog
("Work details" without times, "Searched once", "Listed files once"). A diff count reads `+12 -3`; the in-place
detail's divider is the tool ink at 20 %. The chat's items stand 14 px apart under 28 px of top padding. Held back by
super rule 2: cindy's running preview of the latest five activities, its narration in the open while running, and its
inner summaries folded by default (exception requested in #5162). Kept from Persona: a failed call's red ✕ and red
output (#4303 outcomes, below), which cindy's rows lack.

Subagent launch receipts, interim reports and final results belong under their launching tool, not in independent
parent-chat message bubbles (#3378). Empty completion pointers update status without creating another report. The
shared wire contract is in `docs/protocol/protocol.md` ("Subagent deliveries as tool results").

**Task cards** (web, #5030; the owner asked that the subagent part of the chat follow cindy's `AgentTaskCard` exactly,
which replaces the earlier grid, recent-steps list and side panel). A subagent call and a workflow call are each one
full-width card where they were launched (`TaskCard`: a bordered `rounded-xl` surface, 12 px side padding): a 20 px
round avatar, a small kind word ("Subagent"), the title on one line, the status word with its mark (Running, Completed,
Failed, Stopped; only Failed is red), and a second line in the tertiary color: what ran it, a model chip (the model it
actually ran on, short; never the requested one), tokens, tool uses, the time or "Running for …" each second. The head
opens a subagent card in place (prompt, the latest result folded past four lines, last tool, output file), remembered
per card; a workflow card's head opens the chat's Background dialog at its run. A subagent is never a conversation of
its own (no panel, no page, no sidebar row). Like every tool call the cards sit inside the work summary, running or
not (super rule 2; cindy's running cards outside it wait on #5047). Nested subagents are flat cards by start time. A card offers Stop only
where the node can stop that task. Android draws these cards in Glass (#5045; Brief and Board keep their tool rows with
"View sub-agent"); iOS still draws the earlier design (#5045).
A child session (a Codex subagent, an Agent a Paseo Agent created; #5120) is the same card in its parent's chat, built
from its own row and placed by when it started, since no call in the parent's transcript names it: what runs it, its
model and effort, its status and time; opened, its prompt, latest words and last tool. iOS and Android follow in #5140.

**The plan card and pill** (web, #5156; the owner asked on 2026-11-12 that the plan follow cindy's `InlinePlanCard` and
`TodoListCard` exactly, which replaces the one-line card and the side panel's Plan). One card per exchange, where its
latest TodoWrite of two or more items was written (`PlanCard`; one rule, `isPlanList` / `planCardStep` in
`lib/work-groups.ts`, for the card and the pill); earlier updates and lists of one item are ordinary rows (cindy drops
them; super rule 2 keeps every call inspectable), and a plan update keeps its group from being titled as reading only.
A `rounded-xl` (12 px, as `TaskCard`), 1 px `line`-bordered `surface` card; its head (`px-3.5 py-2.5`, 8 px gaps) is a
14 px chevron in `tool-ink` that turns a quarter when open, a 16 px `todo` icon, done / total in 13 px monospace medium
(as a tool row's counts), a `tool-ink` dot and the step in progress (else the last) on one line in
`--chat-secondary`. It starts open on the whole list, which opens and closes with `Collapse` like every fold in the
chat (rows at least 30 px, 10 px gaps, `--chat-secondary` at 1.45 that wraps):
done steps an 18 px check in a circle with their text in `tool-ink`, the step in progress an arc on the same circle in
`ink` with medium text, pending steps an empty circle in `ink`. The arc turns only while the turn runs and, unlike the
spinners, never with reduced motion (its shape already says "in progress", as in cindy). Closed, a 2 px bar under the
head: `line` track, `ink` fill of done / total, moving over `MOVE_MS` with `EASE`. The card stays inside the folded
summary (super rule 2; cindy draws it in the open, #5162).
While that card is out of view a pill sits above the composer: rounded-full, 1 px `line` border, `surface`, 14 × 6 px
padding, a still 16 px progress ring (stroke 2, `tool-ink` at 45% under an `ink` arc of step / total) and "Step 3 of 5"
in 13 px. Hover shows a flyout and leaving hides it; a click pins it, and a second click, a press outside or Esc unpins
it. Pinned, it is a non-modal dialog on the overlay Escape stack (`useEscapeClose`): Esc closes only the topmost
overlay, and the chat's Esc, which stops a running turn, leaves it be. The flyout is a new pattern: a `rounded-xl`, bordered
`surface` panel centered on the composer column, 8 px above the pill, 220–420 px wide (and the screen less 32), its
list scrolling past 280 px (30 px rows in `--chat-secondary`, the step in progress semibold), coming in with `pop-in` (160 ms, `EASE`, as the other popovers) and
fading out over `EXIT_MS.card`; it is drawn on the page's body so the scrolling box over the composer does not clip it,
with a 12 px band over the pill's top so the pointer can cross the gap. Its 28 px round ✕ (**Close Plan**, `tool-hover`
on hover) hides the pill until the plan changes. The pill also goes once every step is done or a later message of the
user's starts another turn.

A tool row names its tool in words on iOS, the web and Android (#2986; Android `ToolNaming.kt`, #4339): Claude Code's tools have verbs of their own, another
server's MCP tool reads "<Server> · <Tool>" ("GitHub · Create issue"), and any other name is humanized. What the call is
about follows, picked from its input by the field that says the most (`ToolNaming.subject`; Android, whose rows show no
verb, puts it under the name where the tool's own icon doesn't say it). Two subjects are words of their own: several
findings by their count ("2 findings") and a wake-up with no reason by its delay ("5 min"). A computer tool names the Mac
as the user knows it ("MacBook", not the `macbook` the tool wrote). Opened, a tool
no table knows shows its arguments as `key: value` lines, and a tool that operates a Mac its steps in words ("Click AC →
6 → ×") with what the driver answered and the picture it brought back, each with the JSON one tap away under Raw Input
(`RawInputToggle`; Android draws both on Glass, `ToolArguments` and `ComputerToolDetail`, with the same inline toggle,
and Brief and Board keep the input as code with the reader's Raw input button). The cases both platforms follow are in
`spec/fixtures/tool-names`. Details in `docs/lessons/ios.md`.

A tool row says what its call came to when the node sends `outcome` (#4303, `docs/protocol/protocol.md` "Tool
outcomes"). Only a call that ran and failed turns the row and its chip red, and its result says "Exit code N". A call
that was denied, refused before it ran or interrupted gets a gray mark with its word (Denied, Did not run, Interrupted)
for VoiceOver, as section 4.8.1 keeps red for what is still wrong; one that timed out says Timed out. Under an output the
page cut, the result shows the output's end (End of output), where a build or test run says why it failed, before View
full text. Without an outcome (an older node) a row draws as before.

### 4.7.1 Opening and closing in a chat keeps the tapped row in place

Anything that opens or closes inside a chat, in Persona's conversations and Agent chats alike (work lines, tool steps,
thoughts, notes, task cards, long quotes, a report's Expand, and so on), keeps what was tapped where it was on screen,
so the conversation reads on without a jump (#3757). The reference point is the tapped row's top edge, or the tapped
control when it sits lower in the row.

- **In the middle of a conversation**: the tapped row stays at the same screen position, within a point or two, from
  the tap until everything it opens has arrived, including steps fetched from the hub a moment later. What opens grows
  downward under it; what closes takes its space with it at once.
- **At the newest end, following**: the list stays at the newest end and what opens grows upward, the tapped row rising
  with it, until the row's top reaches just under the top bar. There the list stops following and ↓ shows, so the opened
  row is never pushed off the screen. Closing there brings the newest end back: the row comes down by what closed,
  gliding, not in one frame.
- **At the oldest end**: what opens grows downward and nothing moves above it.
- **Your own long message** (#5676, `BubbleFold`): its top stays where it is when it opens or folds, at the newest end
  too, with no rise to the top bar: the owner asked that the bubble never move (2026-11-12). What opens runs on below
  the screen and ↓ shows; folded from its end, what was under it comes up (`toggleInPlace(rises: false)`).
- **Afterwards**: the list holds still while the conversation goes on (new steps, the answer, the next turn, a strip
  appearing under the list), until the user scrolls, taps ↓ or sends (section 4.7). A row opened at the newest end
  takes what comes in next the same way it took what opened: it rises until it is under the top bar, then stays.
- **One mechanism**: rows report their top (`chatPinned`, `WorkHeadReport`) and change through `toggleInPlace` or
  `toggleWork`; `ChatPin` keeps the tapped row in place, waiting for what comes from the hub (`ChatPin.awaiting`), and
  `ChatFollow.toggledRow` decides how it is held. A new expander uses these; it never animates its own height or
  scrolls the list itself.
- **Tested** by `KeepTappedRowTests`: in the middle, at the newest end and at the oldest end, one open and one close
  each, in both chats, asserting the tapped row's screen position frame by frame.

- **Edit diffs use the existing side panel (#9160)**: an edit row opens a Changes panel at that file and call, not a modal that hides the conversation. The panel gives the exchange's edited files a tree and keeps the selected diff beside a scrollable conversation on wide screens; a scope switch reads the whole session, including folded history. Live edits never move the reader to another file: the current diff appends in place, and other files use a new-changes pill. The old modal had no room for that navigation and could not leave the explanation and diff visible together; the existing panel already supplies the container, narrow-screen return and session memory, so no second viewer remains.

### 4.8 Pending requests

Task decisions on the web reuse the task-routing chip for **Answer in chat** (#5929). It identifies the task and
states visibly that the next message is saved as the answer; its existing cancel action switches back to ordinary
discussion. The original decision round travels with the answer. Only a delivered and recorded answer removes the
pending card; neither navigation, an unrelated chat message, nor an answer to a different Agent question does.

Expired assistant questions (#9731) keep one home on their question to-do. The shared `ask` payload puts them in **Needs you**, not **In progress**, without inventing an Agent or supervisor. On the web, the existing question card displays the original choices and descriptions inside that row: a single choice sends on one tap, while several questions or multiple selections keep the existing complete-form submission. Its answer goes through the to-do's late-answer route, not the expired interaction. **Later** defers only this browser's current question round; opening the to-do preserves its existing explicit close/drop controls. A failed answer keeps the original form usable. This adds no style, typography token or pending-request list.

An irreversible deadline only reminds the owner: the question stays in **Needs you**. A reversible default reserves its answer while being delivered or recovered, so it never offers a second decision; a failed default returns that decision to the owner. A rejected inline answer retains the same selections for retry and never shows **Answer recorded**.

Kinds: questions, plan approvals, tool approvals, secret entry, MCP forms, and waiting for a reply.

- **Home**: the bottom of the session page, a stack of cards above the composer, which stays usable (iOS `PendingCards`;
  Android `DecisionStack` and `DecisionDock`, #4210 T6). A request that blocks the Agent stays visible there until it is
  answered or folded, and says why the Agent waits ("Answer this so Claude can continue"). A card its session does not
  let the app answer (no `capabilities.approve`) is greyed with one line under it in `cautionText` saying why: where the
  session can be attached live (a Codex thread on a shared app-server not joined yet), that attaching answers it here,
  with the ⋯ menu's **Attach Live** beside it; otherwise, to answer it on the computer (iOS and iPadOS, #6868; Android
  says the same inside the card; the web still greys it without a word). The cards stack as they come
  while they fit in half the page; past it they scroll on their own under "N pending", permissions first and then the
  newest, and a new card brings the stack back to its top. "N pending" counts the cards in the stack. With the keyboard
  up the cards take the conversation's room above the rest of the footer, the composer staying over the keyboard, and
  "N pending" is left off a lone card. One exception to the half page (iOS and iPadOS, #6277): a lone question card,
  with the keyboard down and no voice call, may grow past half the page until the conversation keeps
  `Theme.Size.cardThreadRoom` (72 pt, about two lines of a message) in view under the chat's header as it stands open
  (more once an Agents header has folded to its slim bar), and never less than the half page it had. On an iPhone 17 Pro
  a question with three options with descriptions then shows whole, with 54 pt to spare. What still does not fit scrolls
  inside the card: on iPhone SE at the default text size the third option's description and Other, a fourth described
  option in a card of several questions, the largest text sizes. The card is laid out as fitting (no "N pending", no
  scrolling stack). The cap is made only from values the card does not move (the page, the open header, what the footer
  holds besides the cards), so it settles at once. Several cards, the keyboard up and a
  voice call keep their rules above. Each card folds on its own: a tap on its header folds it to one line, its title and
  what it asks, and another opens it; any can be folded to answer later. A message typed while a card waits is sent as
  any other, also while a permission holds the Agent (iOS sends it the same way). Each style keeps its own card: on
  Android, Glass a floating card inset like the composer, Brief a rounded card with a light shadow, Board a bordered card
  under its amber rule.
  A Persona conversation's own requests (its model's tool approvals, `ask_user`) are the same: the stack above its
  composer, under the Needs you / In progress entries and the route chip, on every client (#4883: on the web and Android
  they sat only in the closed Needs you tray, and an owner waited minutes on an approval no screen in the chat showed).
  Needs you still counts them, and its row for one closes the tray onto the card instead of drawing it a second time.
  On the web, a card the conversation's own turn waits on (its own session's request, not one of a worker or side
  session folded under it: `lib/turn-asks.ts`, #11794) is never folded: each stands open above the Needs you / In
  progress capsules, all of them at once, and Needs you neither counts nor lists it again. The cards of sessions folded
  under the conversation keep the stack and the tray as above. iOS and Android keep the stack for both until they follow.
  The stack holds the conversation's own cards alone, as one slot on the web: its first card with "2 / 6" (where it stands in the run), the next taking its place when it is answered, and what was just answered one line over it that goes 3 seconds after the last answer (details with the interaction cards in `docs/web/web.md`).
  What the conversation's tasks ask is a card of its own above the entries (iOS `TaskQuestions`, #4339 C10): the oldest
  question of their Agents, under its task's name with the ask mark in the needs-you tone and, at the end of that line,
  "N more" for the tasks' other questions; it is answered there, and the next comes up once it is. The three numbers
  count three things and say so: "N pending" the conversation's own cards in the stack, "N more" the tasks' other
  questions, and Needs you its rows (one for each task or Agent that asks, plus the conversation's own cards, plans and
  drafts). An open plan is also a card among the conversation's lines (iOS `FeedLine`, web `FeedLineView` #8193, Android on Glass `GlassPlanCard`), at the
  end of the exchange it happened in: its tasks, where each would run, "Start these directly", then Start, Edit and Don't start;
  once started the hand-off cards take its place, and a dismissed one reads "Not started · N tasks". A task's or an Agent's row in the tray opens that task's or Agent's detail, as iOS's rows do, where its page
  and its cards are; it never brings a card into the stack, which is the conversation's own. The two places of cards
  share the room the footer leaves above the composer: on Android each takes at most half the page, and with both on
  screen each has at least half of that room, a card that needs less leaving the rest to the other, so the composer
  stays on the page. iOS (#9392) caps the task question as it caps the stack (`FooterRoom.cardsRoom`): alone, as a lone
  question card (#6277), with the conversation's own cards on screen half of that room each; its card fits itself to its
  share and scrolls inside, its task's name and title row in view. It was uncapped, and on iPad and the open iPhone Duo
  it took the whole page, its top under the top bar.
  A question card folds from its title row (the whole row, and the fold button at its end, `interaction-fold`) to one
  line: its title, the question and, for a card of several questions, "N left". Folded, it keeps its page and its picks
  and gives the conversation its room. Where the chat finds the message the card is about (the agent's words before the
  call that asked it, `CardSource`), the open card's title row has **View in Chat**: the card folds and the chat goes to
  that message and lights it up, as a quote's jump does. A task's question asks in another chat and has none.
  A child session's request (web, #5120: `folded_under`) has its parent's page as its home: shown there with the
  parent's own, "From subagent · <title>", counted once under the parent, and every link to it opens the parent.
  A lead's worker's request (web, #5180) has the lead's page as its home too. While the Team panel shows that worker,
  the request is in the worker's conversation there and the lead's dock leaves it out, so the page never shows it twice.
  Otherwise it is in the lead's dock, "From worker · <title>". It is counted once under the lead, and a link to it opens
  the lead's page with the panel on that worker.
  A side session's request (web, #7944) has its parent's page as its home the same way: while the Side sessions panel
  shows that side, it is in the side's conversation there and the parent's dock leaves it out; otherwise it is in the
  parent's dock, "From side session · <title>". It is counted once under the parent, and a link to it, its
  notification and its Needs you row open the parent's page with that side shown. A request of a row that belongs to a
  side (`folded_side`, its own Codex child) goes with that side.
- **One count, per session**: the Agents list's Needs you section, its Needs you filter and the tab's badge count the sessions that wait for the user, each once, its requests in its row oldest first, the way `docs/assistant/managers.md` ("Counts") counts an assistant's waiting agents (principle 4). The Inbox's Agent section and the Assistant's composer entries open the same requests; the composer's entries count tasks (`docs/assistant/unified-entry.md`).
- **Background rounds and supervisors** (#4314, `docs/assistant/assistant.md` "Background rounds"): in the main conversation a round another conversation, a scheduled task or a follow-up started reuses the quiet supervision record, one muted line `@<source> · Background round` that always opens, never a new component; a round that asked or told the user stays shown. While that switch is on, the composer's Needs you groups rows under the conversation supervising them, this one's first, with a plain caption header and no other change: the same rows, order within a group and count.
- **Cut off** (web, 2026-11-14): the dock's cards over the composer keep to their cap (55% of the visible height, #5375) and
  scroll; a card whose answer row stands at the box's bottom edge while the end of its body is under that row or below
  the box says so in that row: its title and what it asks (the command) beside the answers, the whole of it as the
  tooltip, a click bringing the whole card into view with the focus on it. A card that asks only its title (a question)
  says it only while the title is out of view too: the line never repeats what is in view. While any card of several is
  cut off, the first says **N pending** at the end of its kicker row (the room that row already has), on a narrow screen
  once the stack is opened in full (folded, its "N more" row says it), so the stack never hides how many it holds.
  Wherever an Allow is in view, what it allows is in view too.
- **Room** (web, #4656): an open tray of requests takes the room its cards need, up to most of the visible height (the assistant's Needs you / In progress tray lets the dock grow to 82% of it while open), never a fixed short box. Its header (title, All, Expand, Close) stays and only its list scrolls; a card's answer row stays at the list's bottom edge with a hairline over what scrolls under it. **Expand** in the header fills the chat column for going through many requests, the composer still on screen; the same button, **Restore size**, or closing the tray gives the room back.
- **Card text**: the title names the request and what happens next, in the same words on every client: the request titles in section 5.6. The body gives only what the decision needs: the command, the plan's summary, the question and its choices. Commands render formatted, never as raw JSON. A script ask whose interaction has `format: "markdown"` draws its permission summary or question with the existing reply Markdown renderer on iOS and the web, including inline styles, lists and short tables; missing and unrecognized formats keep the literal card text (#10857).
- **A plan's recorded following mode** (#3784): when the plan interaction carries `plan_then`, the existing approval card names that mode with the general permission-mode label and offers one approval, not a competing set of modes; the recorded mode came from the task's start, so it wins on older clients too. Send Back stays. A card without the field keeps the existing approval-mode choices and default. This changes only the affected approval's content, reusing the current buttons, menu-free action row and localized labels.
- **Editing before Allow** (web Lark send/reply): the default card remains read-only with Allow primary. **Edit text** and **Deny with note…** progressively disclose their fields only when the server declares the capability. An actual edit hides Always and makes Allow apply to the reviewed replacement, never the original command; blank text disables Allow. Deny remains an explicit action, with its optional reason secondary to the decision. Other clients and other permission kinds keep the unchanged Allow/Deny layout.
- **One scroll, the answers in view** (iOS and iPadOS, #6605): a card scrolls in one place at most. A lone card fits the
  cards' area (half the page), so the area never scrolls under it; several scroll in the area, as above. Inside a card
  what it asks (the tool, the question, the agent's words, the command, the plan) is in one scroll area and its answers
  sit under it, in view without scrolling: Deny / Allow and Always, Yes / No and up to four short choices of a terminal
  dialog, a plan's buttons, a question's Previous / Next / Submit (#4839), Send in Beautiful UI's card. A long command,
  a terminal's text under its question and a plan fold to their first lines (eight; six for a plan in Beautiful UI and a
  few for its commands), the last one fading as a long message's does, with **Show All N Lines** (a plan: **Show Full
  Text**) under them in the fold control the chat's long text uses; it opens the whole text in a full-screen sheet where
  it scrolls and copies (the chat's full tool output sheet, the plan's sheet). When the card is short (a small phone, the
  largest text sizes, the keyboard up) the fold shows fewer whole lines, never fewer than two, and the answers keep
  their place; only past that does what it asks scroll. Before, a long command scrolled in the card and the card in the
  area: two scrolls, the answers last. A question's options with descriptions, more than four choices or check boxes
  scroll with the question, as question cards do.
- **A file tool's change** (#4621): an approval for Edit, MultiEdit or Write shows the file and the change as the chat draws
  the call once it runs (`PermissionDiffView`: the file header with +N −N, then each edit's `DiffView`), eight rows of each
  diff until Show All and four at the accessibility sizes, so the card's choices stay in view. A card that carries only the
  path (a Codex edit read off its terminal) shows the path as before.
- **A command asked about on the terminal** (#5266): a permission prompt the hub reads off an Agent's screen (Claude
  Code's own safety check in bypass mode, "Bash command … Do you want to proceed?") is a tool approval, titled as one
  ("Claude is waiting for your approval"), with "Claude wants to run a command" in place of the terminal's question. Its
  body: the agent's own words for the command, the command in code type (on iOS folded as above, eight lines until Show
  All N Lines, #6605; the web shows four, and the toggle only when the rows cut something), the warning in plain words ("Can't confirm this command won't
  delete files", a generic line for a warning there are no words for) with Claude Code's own warning and countdown under
  Show original, and the countdown as a line that runs ("Denied automatically in 1:44", then what happens when it ran
  out). iOS draws the time with `Text(timerInterval:)` and switches to the words for the end after one sleep until then,
  with no timer of its own; the web redraws the line once a second while it is shown. The prompt's Yes / No are Deny /
  Allow (primary); a prompt with more choices keeps their labels as numbered rows. Beautiful UI's card keeps its choice
  rows and Send (as its permission card) and is tighter (`compact`, #5556), so Send stays in the half-page pending
  area's view: the agent's words in the secondary type close over the command (as that card's reason), two rows of the
  command, the countdown beside the warning where the row has room. Components: `ScreenCommandAskBody`,
  `ScreenCountdown` (iOS, default style and Beautiful UI), `CommandScreenCard` (web
  `web/src/views/chat/CommandScreenCard.tsx`, its own chunk loaded when the first such card shows; until it is in the
  card is the terminal's question as before); the rules in `apple/Shared/CommandAsk.swift`, `web/src/lib/command-ask.ts`
  and `web/src/lib/command-ask-lines.ts`.
- **Buttons**: for a tool approval, in this order: Deny, Always Allow for This Session, Allow Once (primary). High-risk actions offer no "always" and ask twice.
- **Secret hand-off (#3962)**: the card that hands a provider's secret values to one Agent run is a permission card with the tool
  approval's title ("Waiting for your approval") and buttons (Deny, Always allow, Allow); its body names each variable with the
  item it comes from, the Agent and its computer, where it runs and what it is asked, never a reference or a value. Like the
  password card it is answered on the card only: no quick answer on a row, a notification, the Live Activity, the watch or
  Siri, and no key approves it (`docs/extensions/secret-providers.md` "In the apps"). Android uses
  `SecretHandoffSummary` and `SecretHandoffActions` inside the existing chat `DecisionDock` and Agents request sheet;
  only those card actions send the request-specific `secret_handoff` feature, while its notification and ongoing-status
  surfaces open the app without an approval action.
- **Task-review decisions (web, #4808)**: reuse the answerable decision row, its recommendation tag, the `Button` action group and the draft's Later / Don't handle labels. The two secondary actions stay on a separate footer so they never displace the recommended choice; the footer follows the tray's scrolling edge and uses the existing 44 px button minimum. Later defers only this browser session and this review round. Don't handle persists a dismissal of this exact review, offers receipt-scoped Undo, leaves the task open and takes no option; a dismissed review cannot take its timed default. A new review is visible again. The web hides the row and updates its count optimistically; a confirmed same-round dismissal stays hidden across reads and reloads, while a failed write restores the row with an error. An old Agent runtime error does not override dismissal or Later for that review; an actual Agent input request keeps its home visible. Old reviews without an acceptance ask initially show their original explanation and these same controls, never invented choices or an acceptance button with a made-up round. Once a stale decision goes back for rechecking, or an undecidable old review goes back for restatement, that round leaves Needs you and cannot be answered until a fresh supervisor review re-issues it; a fixed-response round keeps its existing visibility. This is a requested correction to the existing row, not a new style or setting.
- **Independent session-review results (web, #1808)**: an advanced Agents action, explicit and progressive: ⋯ reads availability only when opened, and the existing screen is unchanged without the `session_review` feature. Starting gives immediate local feedback before the hub's result. The finished review reuses the pending-card area and question wire shape, with `tool_name: "persona_session_review"` so older clients need no new kind. Its kicker is the number of findings parsed from the reviewer (not the number of changed files); the full report uses the existing Markdown body and one bounded scroll, and the existing `Card` / `Button` patterns provide **Send to agent** (primary) and **Dismiss** (quiet). There is no Other field, notification quick answer, or hand-back action. Send resolves the source card and the manager forwards the persisted report; Dismiss resolves without a message. Failure leaves the same card answerable. The result alone does not disable a later review; another pending answer does.
- **Questions**: one at a time, with single choice, multiple choice, Other and voice. An unsent answer is kept as a draft per request. On iOS, a long assistant `ask_user` or hook question form's choices (or its review) own one vertical scroll while Previous / Next / Submit stay outside it and visible in the actual pending-card area, in both interface styles. One such card never needs a second, outer scroll; that is reserved for moving between several cards. Dragging through choices only scrolls and never picks one. When the interaction advertises `can_cancel`, a visible **Skip** text action with at least a 44 pt target leads that action row, on the review too; it is not the card's fold mark. Skip sends only `{"cancel":true}`, is disabled while an answer or another Skip is in flight, and leaves the card and its picks in place with the error and Retry on failure. Success removes the card and gives the receipt **Question closed without an answer**. Persona's own main-conversation `ask_user` has no terminal handoff; an Agent's question keeps it. Cards without `can_cancel` have no Skip and keep their existing layout.
- **A card this client can't answer** (web #7497, Android `approvalBlockedReason`; iOS in #6868): the session's
  capabilities withhold `approve` (a Codex thread Persona hasn't joined, a subagent its parent drives, an unknown
  session), so the card's answers are disabled. It never greys silently: one line under the card in the attention tone
  says why. Where the session offers a live link: "Link it live to answer it here, or answer it on the computer", with
  the ⋯ menu's **Live link** (same label, icon and action) beside it; once linked the card answers and the line goes.
  Otherwise: "This session can't be answered from here. Answer it on the computer." An answer on its way is a different
  state and keeps its own "Sending" line in the card's answer row. A manager work card (`persona_triage`) has no line:
  its local defer and dismiss still work there. Web: `answerBlock` (`web/src/lib/format.ts`); the line and Live link
  load in their own chunk, off the first screen (`web/src/views/chat/CardBlockedLine.tsx`).
- **A question the assistant sees first** (`delegating`, then the same `id` with `delegate`): while the assistant decides, the question stays answerable but makes no arrival sound, haptic or notification; the handover replaces it by `id` and announces once. The card shows what the assistant filled in (`what`, `why_now`, `recommend`, `cost`, `reason`, `deadline`, in that order). A recommendation in an option's own words tags that option **Assistant's pick** without selecting it; another recommendation stays in the context. iOS `DelegateNotes`, web `QuestionCard`, Android `DelegateNotes` in `feature/chat`.
- **A question the assistant answered** (iOS `DelegatedCard`, web `Delegated.tsx`, Android on Glass): no longer a pending request, so it is counted nowhere and has no Needs you row. The newest one for the Agent is a card over its chat's composer, after the Agent's own cards, until its × puts it away: "Answered by the assistant", each question with the answer as it is now, "Why: …", and Change Answer while the chat takes messages. Change Answer's sheet starts from the answers so far and sends only what changed, as a message to the Agent. Session Info lists every one with the same body and Change Answer.
- **Everywhere at once**: the first decision on any surface (phone, web, notification, watch) wins. Every other entry updates at once, with a light haptic, and offers Undo where the action can be undone.

### 4.8.0 Task cards (web, #5569)

The task cards follow `docs/design/task-cards.md`, which holds the state tables and the reasons.

- **One card per task, when it is handed out** (#8237, replacing #5569's conclusion cards): the hand-off card is a task's only card in the chat, and it updates in place: its state word follows the task, and a start that failed or a turn the model service cut off reads as such on it, in the danger color, with why in plain words (a raw error stays in the task's details; one naming no known cause reads "Didn't start. Open it for details."). Receipts (done, stopped, failed to start, interrupted), reviews and reports are not drawn, old ones in the history neither, and nothing in the chat says where progress went. The task's row says it: In Progress and Needs You below. iOS, the web and Android since #8237 (Android: #8312). A failed start's and an interrupted task's Needs you row keeps `--danger-wash` with `--ink` words, as the conclusion cards had (contrast in D10 of `docs/design/task-cards.md`).
- **Rows that act** (#8237): a failed start's row in Needs You carries Retry and Remove, an interrupted one Continue (so does a usage limit nothing continues by itself, in In Progress), right under its words and aligned with them, as buttons of their own, never inside the row's tap. A tap elsewhere on the row opens the task.
- **Finished** (#10922, replacing #8237's Just Finished group under In Progress): an entry of its own after the unfinished work's (on the web after Parked and Persona reviewing; zh-Hans 已完成, ja 完了, the To-dos page's word for Done) listing the handed-out tasks that finished in the last 24 hours, seen or not, newest finished first: a row each with the done mark, the title, Done and what it did (its accepting review's note, why it closed, its receipt's summary, else its Agent's last words). A row nobody has opened since it finished (the To-dos page's Just Finished) is new: the unread green as a plain dot and the state word Just finished, as the Agents list's Just finished rows. Opening a row opens the task and counts as seeing it; it stays, as Done, until its 24 hours pass. The entry is there only while it has rows or its tray is open, drawn quiet (secondary words; the done check's green as its mark on the web and Android), and its number is its rows: finished work, never added to the open-work counts of section 4.8. Why not keep the group: it listed only the unseen rows, so opening one took it out of the tray and only the board had it. Why 24 hours: the window the To-dos page keeps Just finished by, so tasks have one "recent". The entries' row scrolls sideways when they do not fit, on every client (iOS since #10922).
- **Accept and decision rows** stack when their container is narrow (a container query, not the viewport): the words take the whole width, the secondary buttons one row, the primary its own full-width line. Never a menu.
- **Progress rows.** The status word is its own right-aligned column, then a fixed end slot: `›` where the row opens something, a check once finished, an info mark for a usage limit. A task whose Agent is not up says Starting. A usage limit's second line says when it resets. A stopped task that is not done says "Stopped, not done yet · <why>" from its completion check (#3180; iOS too since #8237), else its Agent's last words. The column and the row's leading mark say only what the task's Agent itself is doing (#6492, `web/src/lib/task-row-status.ts`): Working, Needs you, Done not seen, Idle, Stopped (its lead session is gone), Interrupted, Usage limit, Couldn't start, Starting (only while a start is in flight, #7594; a to-do nobody dispatched says Not started); the mark is that Agent's `StatusMark`, the one mark set for Agent states. What the to-do itself is at (parked, waiting to merge, awaiting review, rework, the owner's call, accepted, cancelled, waiting for Start) is a chip after the title, "To-do: Parked"; a to-do with nothing to add has none. Before, one column mixed both, so a parked to-do whose Agent had stopped read Parked.
- **Needs you rows with something to answer are folded to one line** (mark, title, what it waits for, the kind) and open in place on a click of the line or its chevron (`aria-expanded`); the card leaves with the answer, and every row is folded again when the tray changes. The conversation's own cards stay in the chat, and their rows close the tray onto them.
- **Persona reviewing rows.** The supervisor-review tray names whose turn it is: its status stays "Supervisor reviewing", even after the task's Agent stops. Its neutral follow-up mark is not an Agent-stopped mark. Finished rows keep their completion mark and label; open rows in In progress use the Agent-only column described above. The separate Waiting for merge group keeps its PR blockers.
- **To-do state chips** use at most half the title area and truncate, so their text cannot hide the task's title on a narrow screen.
- **Hard-coded font sizes.** `text-[13px]` and its kin may only fall: `scripts/ci/web_font_ratchet.py` (`docs/operations/testing.md`).

### 4.8.1 Records of interruptions

A record of something that went wrong (an interrupted task, #3834) says what became of it once that is known: a grey line with the result ("Cancelled", "Dispatched again", "Resumed", "Accepted", "Done"), never a red card that outlives the problem. Several records with one cause in a short time are one card that counts handled and pending and opens to each item. The danger color is only for what is still pending. The hub works out the result and the grouping; the clients only draw them (principle 4; `docs/assistant/unified-entry.md`, "Interrupted receipts after the fact"). In an assistant conversation an interrupted task is not a record in the chat any more (#8237, section 4.8.0): its hand-off card says it, and its row in Needs You offers Continue.

### 4.8.2 Guided setups (step by step)

A setup that takes the user through several things, some of them outside the app (a cable, a restart, a computer), goes
like the iPhone's guided setup (#8199, `IPhoneSetupView`, docs/extensions/iphone.md "Guided setup"), built from the same
pieces as the computer page's install guides (`.card()`, `.themedButton(prominent:)`, numbered steps):

- **An overview first**: what it does in one sentence, what it needs, each step with its state, the safety rule, one
  button (Set Up, or Continue once some steps are done).
- **One screen per step**: a progress bar of the steps, "Step n of N", the step's title and one sentence, a card with its
  state, and Details folded for the technical words. One primary button at the bottom; a step that waits on the
  world has none, or a secondary one (Cancel, Check Again).
- **State words from `StateName`**: Done, Working (`taskWorking`), Needs You, as chips. Every state comes from the hub's
  answer, never from a tap: a step done elsewhere is skipped, and a screen moves on by itself when its step is done.
- **Asking again only while waiting**: every two seconds while a step waits on something outside the app, and only while
  the page is in front (`.tabActiveTask`).
- **The page the user comes back to** is the same page: its switches (turning it off there), a way to try it, and at
  most one optional switch.

### 4.9 Sharing a conversation publicly

Sharing an Agent session follows the user-control rules: everything that widens exposure (more messages, work
details, future messages, a longer lifetime) goes through a preview and an explicit confirmation of possibly
sensitive content, plus a transport-risk confirmation on an HTTP sharing origin; narrowing and revoking need no
preview, and revoking is a destructive confirmation. Defaults are the narrowest: future messages and work details
off, three days. A paused publication can retain its future-messages preference; resuming it is an explicit action
that requires a fresh preview and sensitivity confirmation, plus transport-risk confirmation on HTTP, even when
the preference is already on. Sharing lists describe effective publication: new messages are added only while
the link is active, publication is live and future messages are enabled; paused, revoked and expired links say
that no new messages are added. The editor retains the configured preference separately from publication state.
The link is shown once with a copy action and is never listed again. The public viewer is a
separate, read-only document with the same palette tokens, plain text only and work details collapsed (section 4.7).
Web: `web/src/sharing/`, `web/public-share/`; Android: `AgentShareSheet`; server contract in #3604.

### 4.10 Web workspace selection and management

In Agents' repository tree, the workspace title and its leading disclosure button toggle the same remembered fold at every depth. A title also selects the exact workspace, including an empty one; opening an Agent uses that Agent’s own row, never the workspace title or a descendant chosen by proximity. A visible overflow button offers the same actions as the context menu, including on touch devices. The desktop sidebar and phone Agents page share these rows and actions rather than adding a management destination.

The existing `Dialog`, `Button`, `Menu`, `Collapse` and status components carry workspace management. Creation shows its source and starting branch, keeps additional choices in the advanced group, and starts no Agent automatically. An optional Agent start names the exact existing workspace. Creation acknowledgment shows the actual resulting path and branch; an interrupted request offers reconciliation of that same intent.

The workspace catalog, selection and management actions follow the current assistant. Switching assistants retires outstanding catalog reads, clears the selection and invalidates pending mutation previews, including a switch away and back. Management loads on the first workspace action; its loading menu keeps the same dismissal and phone Back behavior, and operation receipts survive menu reopening.

Removal uses the confirmation sheet pattern in section 4.4 because it needs branch and uncommitted-file choices. The dialog identifies the workspace and explains which files and running sessions will be lost. Keep the branch by default; require an explicit discard choice for uncommitted files. Confirmation stays unavailable until the computer supplies a valid preview. A changed target requires another preview, and an uncertain dispatched removal offers status reconciliation rather than another destructive submission. Renaming uses the existing text-entry dialog pattern and preserves workspace, branch and Agent identities.

Backend naming (#7100) is a single collapsed row inside Assistant settings’
Starting tasks group, directly after the workspace preference. It appears only
for a confirmed matching assistant with `prefer-new`; missing or unknown values
keep it hidden. Hiding never clears saved rules. Todo details retain their own
independent advanced entry. Both reuse `Button` and the standard settings input.
Each mount confines import/render failures to a local `PageBoundary` around its
`Suspense`, keeping sibling editors mounted and saveable. Recovery retains the
existing one-time page reload and explicit Reload behavior.
The editor pairs each label with a wide input and small, always-visible template
guidance; labels stack above inputs on phones. It serves people configuring
coding tasks; ordinary tasks need no new fields. Unset fields stay empty and
show their system default in a placeholder; Todo overrides show the effective
Assistant default instead. Clearing a field and saving removes that override
using the existing null patch, without saving a fallback as a custom value.
There are no inheritance status labels or reset buttons. Preview options remain
folded until requested. The preview result shows the task folder once and then
either the root repository's labeled branch or each container repository's
folder-to-branch mapping, never the internal `.` path and never the same absolute
path twice. The computer and layout selectors and the preview action form one
compact row, stacked full-width on phones; a task's repository choices sit above
that row, and the result carries no routine planning time, time zone or
disclaimer text. A failed save retains its draft; reloading
updates the saved baseline without replacing local edits.
After a task save, refresh its saved revision and effective Assistant defaults
without replacing the draft, computer, layout or remaining repository selection.
If that read fails, keep the acknowledged save and offer the same read recovery.
Changing a field, computer or assistant retires earlier previews. The panel
changes no creation mode and starts nothing. This is a secondary settings
surface, not another workspace management destination.

### 4.11 Questions of several

A card that asks several questions at once shows where it is and lets the answers be checked before they go (a pending
request, section 4.8). On iPhone and iPad (#2970) that is a terminal dialog of several, and a hook question of several:
Claude Code's AskUserQuestion, the assistant's ask_user and Codex's questions.

- **Step capsules** (`ScreenSteps`, `BUIScreenSteps` in `apple/Persona/Views/Interactions/InteractionViews.swift`): one per
  question, ticked once answered, the current one in the accent. They show progress only and are not buttons. When the
  row is wider than the card (the largest text sizes) it scrolls, opening with the current one in sight; the card's title
  then wraps to up to three lines.
- **Previous / Next** move one question at a time. A question gone back to shows the answer it had.
- **A single-select pick** moves on to the next question by itself after a short pause (250 ms), once, and never past
  the last one. A second pick within the pause starts the pause again, so the card moves a whole pause after the last
  pick and its tick is seen; Previous / Next meanwhile cancel the move (`QuestionSteps.step`, #6549; two taps within the
  pause used to skip a question).
- **The review** (`ScreenReview`, `apple/Persona/Views/Interactions/ScreenReview.swift`): a row per question, Previous and
  **Submit N Answers**. A question left reads **Not answered** in the caution color; the main button then goes to it,
  and **Submit Without Them** sends the rest. At the largest text sizes the buttons stack, the main one first, and so
  does the card's last row (Reply in Chat Instead, Cancel, View Terminal); a label that still does not fit then shrinks
  a little (section 6.2's last resort).
- **BeautifulUI's hook card** keeps its own ↑ n / N ↓ counter instead of capsules; a question skipped with ↓ shows in the
  review as not answered, and its main button opens the card at that question with every answer kept.
- **The plan pill folds** while a card asks something, so it never sits over the card's buttons.

### 4.12 Keyboard in web lists and forms

For people who work from the keyboard on the web (web audit, 2026-11-14). Every list-and-detail page and every form
follows these; the To-dos page is the reference (`web/src/views/Todos.tsx`, checked by `Todos.narrow-keyboard.test.tsx`,
`Todos.plan-fields.test.tsx` and `web/e2e/todos-keyboard-layout.mjs`).

- **A list and its detail at a phone's width** take turns (the list is hidden under the detail). A row opened with Enter
  or a click moves the focus to the detail's Back, as a page pushed on iOS does. Esc goes back as Back does, except from a
  field being edited or under a dialog or a menu, which own Esc. Back on the list, by Back, Esc or the browser's Back,
  the list is scrolled where it was and the focus is on the row that was open, a detail opened by a link too. Side by
  side, nothing moves: the focus stays on the row and Esc does nothing. Why: hiding the list with `display: none` drops
  the focused row's focus to `<body>`, and the next Tab starts from the top of the page.
- **Date and time fields** (`<input type="date">`, `time`, `datetime-local`) save once the value is entered: on leaving
  the field or on Enter, once however soon the field is left after Enter, and at once for a pick from the browser's
  picker. Never per typed segment (the "1" of a day "15" is already a valid date), and never disable a field while it
  saves, which takes the focus away mid-edit (`PlanField`). A half-typed field goes back to the saved value.
- **⌘↩ / Ctrl+↩ submits** a dialog or a box from its multi-line field, as the composer and New Agent do; Enter alone is
  a new line, and the Enter that ends an IME composition does neither.

### 4.13 Keyboard and overlays (web)

- Esc undoes one thing at a time, the innermost first: a search field with text clears it (`SearchField`; an empty one lets Esc go on), a menu's rename goes back to the menu, open with its Rename row highlighted and the focus on what opened it, a menu closes onto its button, and then the topmost overlay closes (`useEscapeClose`). A modal closes on Esc from anywhere inside it, not only from its first field, and gives the focus back to what opened it.
- One modal from the keyboard at a time: the ⌘K palette and the ⌘/ shortcut help replace each other.
- Tab stays inside a modal and reaches every control in it, including the ones after a scrolling body the browser lets the keyboard focus (`trapTabIn` in `web/src/ui/overlay.ts`).
- A menu is as tall as the window allows and shows whole when the window can hold it, over its button if need be; only one taller than the window is cut, opening beside its button on a wide screen. A fade at a cut edge says there is more (`Menu`, `data-menu-more`), as the team panel's list does, and a row the keys move to past the cut scrolls into view.
- A choice kept in the browser (appearance, theme color, text size, language) reaches every open tab: the others follow through the `storage` event. A language needs a reload, as in the tab that chose it, and that reload never lands on a draft or an open dialog or menu: it waits for a calm moment, as a new build's does, with a toast that offers it meanwhile.

### 4.14 Starred messages (web, #7524)

People who want to keep a useful message choose **Star** in the existing message menu. A small accent star at the
message edge shows that it was saved without moving the row. While the save or removal is pending, the same position
shows a spinner. Failed actions keep the prior state. **Unstar** removes the snapshot and offers the shared Undo toast.

**Starred** is a secondary page under **Pages**. Its badge counts every saved snapshot for the selected assistant.
The page uses the existing search field, neutral cards, Markdown, and action buttons, with the sidebar button on small
screens. One card contains a title, speaker, timestamp, saved text, and **Open original**, **Copy**, and **Unstar**
actions. Long saved text folds in the card. The original uses the existing transcript jump. The saved text stays usable
when the original cannot be opened.

## 5. Content

### 5.1 Voice and tone

Calm, direct, second person, like a capable teammate. Contractions in English ("can't", "didn't"). No exclamation marks, emoji, "Oops" or "Sorry". The assistant says "I" only in its own chat surfaces (greeting, intro, sample prompts). Elsewhere the app is "Persona" and agents are named in the third person. "Please" only when asking for something burdensome. The hub's HTTP errors break these rules (uncontracted, "please" heavy).

### 5.2 Capitalization and punctuation

- Apple buttons, navigation titles, menu items and alert titles: Title Case. Descriptions, footers, empty-state lines, section headings, accessibility labels: sentence case. Web and Android: sentence case. `docs/protocol/i18n.md` and `docs/apple/sidebar.md` disagree on headings (DS-27).
- Agent states: "Needs you", "Working", "Unread", "Seen" in prose; Title Case only as a button or title.
- One ellipsis character. Use it for actions that open another step and for in-progress states.
- Complete sentences end in a period; labels and titles do not. Settings paths use the single angle quote.
- Apostrophes and quotes: pick one style (Apple en is 94% straight apostrophes with curly quotes; DS-28).
- A heading that holds a name (a space, a computer, a project, a pane) is drawn as written. The web's small-capital label style (`uppercase`) is for label headings only; on a name it rewrites what the user typed ("weather" as "WEATHER"). Web: `Menu`'s `headerAsWritten`, the palette's `sectionAsWritten`.

### 5.3 Error-message structure

1. What happened: "Couldn't + verb + object". Use "Can't" only for something not possible, not a temporary failure.
2. What is safe: "What you've typed is kept."
3. One next step: "Check the connection and try again."

No HTTP codes, key algorithms, CLI flags or raw error text in the primary line. Do not append raw detail after a colon without a plain lead. One generic fallback: "Something went wrong. Try again."

### 5.4 Buttons, confirmations, notifications

- Verb first, one to three words. "Retry" everywhere (not "Try Again"). "Deny" for permissions.
- Delete destroys data; Remove detaches from a list; Discard throws away unsaved edits; Forget removes a pairing or credential; Clear is bulk reset.
- Confirmation: title "Delete this chat?", body states the loss and whether it can be undone.
- Notifications: title names the actor and the need ("{name} needs your permission", "{name} has a question", "{name} finished"); body is a short call to action. One wording per event on every surface, kept in the hub catalog. Never include secrets or commands.
- Permission purpose strings: "Persona does X when you do Y, for example Z." One or two sentences, no CLI text, correct device name, all three languages.

### 5.5 Glossary and localization

| Term | Rule |
| --- | --- |
| Hub | product term; "the hub machine" for the machine |
| Computer | the user's own machine; "Machine" only for list and monitoring entries |
| Node | never in user-facing English |
| Agent | lowercase mid-sentence; "Agents" for the tab |
| Session | what an Agent runs, titled by what it is doing. Not "task", which names the work Persona hands out |
| Persona | the product and the main assistant |
| Assistant | one of the hub's assistants, each with its own chats, inbox and computers (`manager` in code, the protocol and docs); zh-Hans 助手, ja アシスタント. Never "manager" in user-facing text |
| Lark | in en and ja, except where Feishu is a separate option |
| sign in / sign-in | verb / noun |

Full glossary: `docs/protocol/i18n.md`. Languages: en, zh-Hans, ja. Apple strings live in `apple/Shared/Localization/Localizable.xcstrings`; hub strings in the server locale files. Keys with a bracketed suffix are translator-context tags; the English value strips them. Japanese uses no space between Japanese and Latin text (the catalogs do not follow this yet; DS-29). Do not concatenate sentences.

### 5.6 Objects, titles and state words

| Object | Called | Home | Title |
| --- | --- | --- | --- |
| Assistant conversation | conversation | the Assistant page | its title, generated or renamed |
| Agent session | session | the session page | what it is doing, from the user's first message or a rename; never the repository or tab |
| Pending request | Needs You | its session's pending area | the thing wanted: the plan's title, the command or the question |
| Inbox item | message, mail | the Inbox | the sender or group, and a summary |
| Computer | computer | the Machines page | the user's name for it |

Titles of plans, permissions, questions and terminal prompts, the same on every client and screen that names one (cards, list rows, the Inbox, the chat panel's label, the hub's push titles). iOS writes them in title case, the web, Android and the hub in sentence case. A notification may put the agent's name in front; a label that cannot name the agent reads "Has a question" (在问你, 質問があります). The line under a title says what is asked (the command, the plan's title, the question) and keeps its own words; a row with one line puts the title first ("Waiting for your approval: swift test").

| Request | en | zh-Hans | ja |
| --- | --- | --- | --- |
| Plan | Plan awaiting approval | 计划等你审批 | プランの承認待ち |
| Tool or command | Waiting for your approval; {agent} is waiting for your approval | 在等你批准；{agent} 在等你批准 | 承認を待っています；{agent}が承認を待っています |
| Question | {agent} has a question (Claude by default, Persona for its own) | {agent} 在问你 | {agent}から質問があります |
| Terminal prompt | Terminal needs you | 终端等你处理 | ターミナルで対応が必要です |
| A kind the client does not know | Needs you | 需要你 | 要対応 |

Notification titles for a tool approval (the hub's pushes, the web's browser notifications) still read "{agent} needs
your permission" (section 5, "Notifications"); moving them to the card's words is a follow-up of #4883.

- A session's title is its top-bar title and its row's first line.
- Primary screens (list rows, top-bar titles, the Assistant home and notifications) never show raw model IDs, session IDs, paths or engine names (herdr, Orca, Paseo); this is principle 2 applied to Agent work. A row may name the model by its short name.
- An agent an engine runs under the engine's own id (Paseo's providers: Grok, Kimi, Cursor…, whose `agent` is `paseo`) is called by its provider's label (`agent_label`) and drawn with the mark the client has for that provider, else its label's letter; cards, pushes, filters and model pickers name it the same way (#8142).
- Views grouped by location (By Repository, By Machine, By Project) may show workspaces, tabs, worktrees and branches. The web Agents row may also carry an associated terminal tab as the secondary **Tab {name}** metadata described in 3.4; it is not the session title and requires a real tab association. Other location details stay in the session's info sheet, opened from its title, and in tool views (terminal, files, changes).
- State words come from one place per platform, `StateName` on Apple and `web/src/lib/state-labels.ts` on the web; no screen invents its own.
- An Agent page says the agent's state once. On iOS and Android that is the header's title line (dot and words); other
  marks in the header count or describe (the loop and background pill is a count by a neutral glyph, #3719), and only the
  space's tabs color each agent by its state. The web's top bar says no state (#5421, cindy's): the running status bar
  over the composer says it while the Agent works, the pending cards while it needs you, and its row in the sidebar always.
- A row shows its most urgent state, in the order `AgentListOrder` ranks them.
- A waiting to-do is named by what it waits on, never one "Waiting" for all (#11090, web): **Waiting on you** in Needs you's attention color only when it waits on the owner by the sidebar badge's predicate, every other wait and an unknown one ("Paused (nothing needed from you)") in a neutral tone; the words are in `docs/protocol/i18n.md` ("Agent states").
- A failed run has no state of its own. It shows its state like any other run, and its row's line 2 says what failed, in red.
- A Codex opened with no first message (`awaiting_first_message`) has no state of its own either. It is idle, never Needs You or unread, and sends no push.
  Its row's line 2 says "Waiting for its first message" (web `sidebar.codex_waiting` through `rowLine`, Apple `Session.listPreview`).
  Its chat says once, while it has no messages, that Codex is open on the computer and takes its first message below or in its terminal (Apple: the chat's empty state, in place of its generic line; the web: the chat's `codex-waiting` note).
  Once the message goes in, the page moves to the thread's session in the same pane (`docs/integrations/codex.md` "Starting without a first message").

## 6. Dark mode, Dynamic Type, accessibility baseline

### 6.1 Dark mode

- Every color is a light/dark pair; tokens are dynamic colors.
- The Light & Dark choice sets the interface style on every window, so sheets, alerts, the keyboard and web views follow. Windows created later copy it (toast window, drawer host).
- Intentionally dark: watch, Live Activity island, image editor, VNC screen, image viewer chrome. Code colors are the same in both modes.
- Add a view-level `colorScheme` read only for drawn content that a dynamic color cannot express.
- Web: class on the root, set before first paint, plus a `color-scheme` meta.
- Web terminal: Terminal settings › Appearance (Match page by default, Light, Dark) gives the terminal's screen one scheme's page tokens with `[data-terminal-scheme]`, theme color and extension theme included, never a palette of its own (`docs/integrations/terminal.md` "Appearance").

### 6.2 Dynamic Type

**The rule (decided 2026-11-13): the interface stops at xxxLarge; reading content grows to AX5.**

- **Interface** is everything the person operates or scans rather than reads: buttons, top and bottom bars, toolbars, menus' own drawing, the tab bar, segmented controls, filter strips, list rows' labels and values, page headers, the calendar, widgets, a card's controls, settings rows, badges, previews and summaries in lists, search-result snippets. It is capped at `Theme.interfaceLargestTypeSize` (xxxLarge, the largest size that is not an accessibility size). `RootView` sets the cap on the environment's `dynamicTypeSize`, so no view has to ask for it; a view that needs a smaller cap (a glyph in a fixed circle) still names one.
- **Reading content** keeps growing to AX5: a chat message's body (the user's and the assistant's, replies, quotes, and the same in an Agent session; code blocks follow the reply and scroll sideways), an Inbox item's body and detail, knowledge and file preview bodies, a to-do's description and plan, meeting minutes, transcript and the messages in a meeting's conversation, and what is being typed in a text field (the field's own buttons stay capped, its line limit stays). A tool row, a "Worked for…" fold header, expanded tool output, a diff and the terminal are interface (the terminal has its own text size).
- A view lifts the cap with `.readingContent()` (`apple/Persona/DesignSystem/ReadingSize.swift`): it sets the size the reader chose, kept by `RootView` in `\.readingTypeSize`. Put it on the text, not on a whole screen. `scripts/test/test_apple_reading_content.py` lists the files that may use it; adding a file needs the reason there. Outside a reading container a view never sees an accessibility size, so it has no AX-only layout: `scripts/test/test_apple_ax_branches.py` lets only the files it lists mention `isAccessibilitySize` or `.accessibility1…5`.
- The system draws its own menus, alerts, confirmation dialogs, share sheet and keyboard at the system's size; do not cap them. On iOS, an action sheet is the one system-presentation exception to a simple trait cap: above xxxLarge its button capsules can follow the window's cap while their labels still use the application's larger size. `.accessibleConfirmationDialog` therefore keeps the existing confirmation dialog through xxxLarge and uses an alert above it, with the same title, message and actions and exactly one Cancel. This is a presentation choice inside `WindowTextSize`, not a cap on the system's label and not an accessibility-only page layout. visionOS has no window cap and keeps the confirmation dialog.
- Because the interface is capped, it needs no layout of its own for the accessibility sizes. Do not add one: no `isAccessibilitySize` branch and no `.accessibility1…5` in interface code (`PR 2` of #6448 removes the old ones). The shared confirmation-presentation choice above is the only exception, and pages never make it themselves. A control that still does not fit at xxLarge or xxxLarge uses the rules in section 1.2 (shorten, stack by the width that is there with `StackingButtonRow` or `firstFittingText`).
- Large Content Viewer: a capped control shows its name and icon large on a long press (`accessibilityShowsLargeContentViewer`); VoiceOver reads the whole content as before.
- Screenshots: an interface screen is shot at the largest size up to xxxLarge (`simctl ui <udid> content_size extra-extra-extra-large`); only a screen with reading content is also shot at AX5 (`accessibility-extra-extra-extra-large`).
- Text Size (Small / Standard / Large) moves the system's size first and the cap comes after: the interface is `min(moved size, xxxLarge)`, reading content is the moved size. At the system's xxxLarge and above, Large has no effect on the interface (it is at the cap) and still adds a step to reading content; Small always takes a step off both.

- Text uses `Theme.Font` levels only. Shapes use `scaledFrame`.
- A heading's counts and an agent row's time sit beside the name, and go under it when the name would keep less than half the width (`BesideOrUnder`: `CountsHeading`, `TimeBesideOrUnder`); the choice is by measured width, not text size.
- Rows with name plus chips keep the chips right after the name while the name keeps half the width (or all it needs), else put them on a line under it, by measured width at every text size (`NameWithChips`). Use `firstFittingText` when several candidate labels exist.
- A date, or a date and a time, in a form row is a `FormDatePicker` (`apple/Persona/DesignSystem/FormDatePicker.swift`, #6108): the system's compact picker beside its label at the usual sizes; at the accessibility sizes, where the compact date and time are wider than the row and were cut off, the label on its own line and a wheel under it across the row (the wheel's words don't grow; that is the system's wheel). A sheet holding one sets its height with `formSheetDetents`, which opens it full height at those sizes. Used by New To-Do, the to-do's page (Due, Remind At) and "Change Due Date…"; new date rows use it too.
- Caps are allowed for glyphs in fixed-size buttons and for top bars that must fit a small iPhone. They must be named, and the control must keep a 44 pt hit area.
- A bar of fixed items (the message long-press bar, `MessageActionBar`) keeps its items at the default text size (`barTypeSize`), as the system's tab bars and toolbars do, so every word that fits there fits up to the accessibility sizes; each item has the Large Content Viewer (`accessibilityShowsLargeContentViewer`), so a long press shows its name large. Its words are short enough for their items at that size in every language (rule 3 (a) in section 1.2; Japanese 「文字選択」 for Select Text). At the accessibility sizes the actions become a list, one a row with its icon and whole name, growing up to `listTypeSize` (xxxLarge), and the lifted message is placed by the list's measured size (rule 3 (c)); never "…", never an icon left alone (#5019).
- The to-do page's bottom buttons (Hand to Agent, Done / Reopen) are a `StackingButtonRow` (3.7). On an iPhone 17 Pro they sit side by side through xxxLarge in Chinese and through xLarge in English and Japanese, and stack beyond that, Hand to Agent first, each across the page; side by side, those labels were cut to "Hand to A…". The button's Japanese is the shorter 「エージェントへ」 (key `Hand to Agent [To-do Page Button]`), which fits the half-width button wherever the English does; the hand-off sheet's title keeps 「エージェントに任せる」 (#6151). They are pills floating over the notes (`floatingPill` in a `floatingFooter`, 3.7 and 3.12; #9008): Hand to Agent the Theme Color's, the one solid button, Done / Reopen the glass one, 46 pt as the Sidebar layout's New Chat, with no band of page color behind them, the notes scrolling on under a blur. On a wide page (an iPad) their row is no wider than `Theme.Size.readable`, centered, as the Inbox's hand-off bar. At the large control size across the whole page they were 61 pt tall and on an iPad 380 pt wide each; a first fix at 44 pt sat them on a white strip. Android's bottom bar is capped the same way: no wider than `Tokens.Size.readable` (560dp), centered (2.5, #9009).
- Never shrink to hide truncation of body text. For button labels `minimumScaleFactor` is a last resort only; follow rule 3 in section 1.2 (shorten the copy, reconsider the control, stack the group at accessibility sizes).

### 6.3 Accessibility baseline (the contract every component meets)

1. Hit area at least 44 x 44 pt (Android 48 dp), including icon buttons whose visual is smaller. Use `tapTargetFrame`, or `tapTargetHeight` for a row or a heading that is a button, or `tapTargetOutset` for a text link that must keep a line's height (`tapTargetOutset(.all)` for a glyph that must keep its place). This is Persona's rule and is stricter than Apple's: the HIG gives 44 x 44 pt as the iOS default and 28 x 28 pt as the absolute minimum. Persona uses the default as its floor.
2. Every control has a spoken label; icon-only buttons never rely on the symbol name. Decorative marks are hidden when adjacent text says the same.
3. Selected, expanded and busy state is exposed with traits or values, not only color or shape.
4. State never rides on color alone: pair with text or a symbol.
5. A gesture-only action has an accessibility action or visible control.
6. Animations: transitions and press effects respect Reduce Motion; every loop is listed in `ContinuousMotionTests`.
7. Text contrast at least 4.5:1 (3:1 for text of 18 pt or larger, bold text, and non-text marks), verified for each style and theme color. Composite cases (text on a tinted fill) need a test. The exception is iOS's own palette, and Android's Glass with it (11-11): white words on the accent 3.5:1, the accent as a link 3:1, words on the user's bubble 4.5:1, with the system's secondary gray as it is (`apple/Shared/Design/PersonaPalette.swift`, `android/core/designsystem/src/test/kotlin/persona/core/designsystem/StyleContrastTest.kt`).
8. Layouts survive xxxLarge, the interface's largest size, without clipping, and reading content survives AX5 (section 6.2).
9. Transient messages are announced to VoiceOver.
10. Headers carry the header trait; modal sheets trap focus.

Current gaps against this contract are listed in `docs/design/HIG_AUDIT.md`. No test enforces items 1, 2, 7 (composites) or 8.

## 7. Platform consistency matrix

| Topic | Apple | Web | Android | Verdict |
| --- | --- | --- | --- | --- |
| Styles | 5 | 1 | 3 (Glass shared) | different sets |
| Accent presets | 5 | same hex | same set | consistent |
| Roles (danger, positive, ai) | reference | different hex | same hex | web differs |
| Needs-you color | blue | amber | blue | web differs (DS-01) |
| Type scale | Dynamic Type, 28/22/17/17/15/13/12 | px literals | sizes match, no in-app size setting | web has no tokens |
| Spacing | 0-32 with `sm` 6 | Tailwind default | same scale | web differs |
| Radius | 6/10/14/20/22 | 6/8/10 plus literals | 6/10/14/20/22, per-style variants | web and Android differ |
| Min target | 44 | 44 hit area, 28-32 visuals | 48 dp touch targets, 32-40 visuals | violated on web visuals |
| Motion | token set | 150/220/300 ms | Glass: the iOS state and layout springs, the composer's send swap and press | web differs |
| Reduce Motion | environment value | CSS strip, busy exempt | animator scale only | different policy |
| Toast | capsule, top | dark capsule, bottom | capsule, top (`JToastHost`) | web differs |
| Haptics | 4 kinds, 443 sites | none | 4 kinds, every copy plus some actions | thin on Android beyond copy |
| Icons | SF Symbols | custom strokes | Phosphor, by meaning (`spec/icons/meanings.json`) | web has no map |
| Text size setting | Small/Standard/Large | same ratios | none | missing on Android |
| Tokens source | hand-written | hand-written | generated from `spec/tokens` | Apple and Android drift fails a test; the web is unchecked |

## HarmonyOS native structure

HarmonyOS 7 / API 26 uses native ArkUI Navigation, destination titles, lists, menus, sheets, selection controls and
confirmation dialogs. Persona and Agents are peer conversation views in the native title control. Inbox, Todos,
Knowledge, Health, Apps, Devices and full Settings open from the native Spaces sheet; extensions do not add peer
views. The peer switch uses the message/group symbols in native `CapsuleSegmentButtonV2`, with localized accessible names and at least 48 vp per item at every font scale. It does not revert to a text Select. XS/SM/MD use a stack and LG/XL use native split navigation, based on the actual window class rather than a
device name. The expanded master pane starts at 320 vp, drags from 280 to 440 vp and collapses to 64 vp. It has no
permanent third pane or replacement navigation rail.

Navigation owns system avoidance on main pages. Main content adds only its 4 vp business footer; do not copy the gesture
area height into padding. A bottom sheet measures the actual content host against the current system avoidance
boundary after layout and adds only the remaining overlap. Recompute when the host or avoid area changes; an already
avoided host or centered sheet needs zero extra system inset. Never hard-code an inset or repeat it on inner rows. Preserve the system
gesture region and keep the last row and primary action above it.

The 4 vp footer is application spacing, not a fixed HarmonyOS safe-area measurement. Native Navigation paints the primary pane's canvas background through the gesture area while leaving scrolling controls in the safe layout region; a short list's remaining height is not extra inset. Question, plan-editor, context-selection and Spaces sheet content uses 16 vp internal edges plus only the outstanding measured system overlap. The intrinsic-height Agent filter sheet uses 16 vp side and bottom edges and an 8 vp top edge, and measures its own outstanding overlap; centered sheets add no system inset. Chat siblings and wrapped GridRow rows use 8 vp gaps, and same-level feed cards use 16 vp content edges. The compact composer reserves fixed action targets and 8 vp gaps, giving its remaining width to the same editor instead of rounding button slots to coarse columns. Do not merge the jump-to-latest button into the composer surface. Appearance labels and Select controls wrap as whole items. At enlarged text sizes Inbox status uses the Agents native Select pattern. Native Back visibility also determines whether a custom title needs its own leading 16 vp inset.

Conversation detail hides its title only while the matching selected row is fully visible in the expanded master
and identifies the conversation for the same owner. Compact, collapsed, filtered-out, offscreen or different-owner
contexts restore the native title. Child pages keep their own title and back action. A title already provided by the
destination is not repeated in the body. Avoid explanatory subtitles, child controls and duplicate actions; native
control placement, names and state must explain their purpose. Explicit target and consequence text remains
necessary for consequential decisions.

Use the shared native roles and `Tokens`, not screen-specific literals. The System style's light palette uses the
emphasized action blue (`#0071E3`) with white on-action text, text-link blue (`#0064C8`) and a light
own-message bubble (`#DCEAFF`; dark `#233B57`). The emphasized control uses the system's paired light/dark resources.
Status roles stay distinct: needs-you is `#0064C8` / `#86B6FF`, working is `#9B5500` / `#FFB224`, unread is
`#1D7A4C` / `#4CC38A`, and positive is `#237337` / `#30D158` in light / dark appearance. Visible state words or
symbols accompany color. Destructive/error colors are not substitutes for working or positive states.
The standard system typography is 20 fp title, 17 fp body, 14 fp secondary and 13 fp caption, with 24 fp body line
height. Follow the application system font scale up to 3.2×; use 16 vp page edges, 12 vp insets, 8 vp gaps, 24 vp
section spacing, 20 vp business-surface corners and a 48 vp minimum response region. System controls own their
internal dimensions and material. Fixed icon slots use 22 vp symbols, independently of the text scale.
The Persona/Agents symbol segment keeps both native 48 vp targets reachable at enlarged text sizes. Native Radio and Checkbox outlines
use the secondary foreground when unselected; the control background is not an outline color.

System is the default style for a fresh or unset preference. Existing explicit Glass, Brief and Board selections stay
selected and remain available after update. Avatar, Accent, color mode and style are separate. Standard is the
default row density when an older device-appearance record has no density value; saving density preserves the
existing Avatar and Accent fields. Agent grouping (Status, Machine or Project) is a separate owner- and
view-scoped workspace preference, not a device appearance field. Preference restoration finishes before selection
is enabled, failed writes keep the last confirmed value, and changing one option does not reset the others.

Shared native Select controls retain a 48vp minimum target and leave their height to ArkUI content sizing; a fixed 48vp height would clip 3.2× text. See the [native Select sizing rules](https://developer.huawei.com/consumer/cn/doc/harmonyos-references/ts-basic-components-select).

The Agent quick filter uses the existing native Select when the actual pane cannot fit all five full localized segment labels. Native text measurement and the explicit segment padding determine fit; a font scale of 1.5× or above also uses Select. Search and filter keep their compact row at ordinary text sizes. This content-filter fallback is independent of the two-icon Persona/Agents peer control. At accessibility sizes a narrow native selector may ellipsize its displayed value; its complete localized value and menu options remain available.

Native `WithTheme` supplies light and dark system tokens; an own-message subtree supplies readable bubble tokens.
Avatar choices use intrinsic native list groups and Radio controls, with no fixed-height nested list or descriptive
captions. Package images remain bounded, decoded natively and installed atomically. Native system materials belong
to supported controls; the client does not simulate them with another navigation shell. Horizontal half-fold uses
`FoldSplitContainer`: reading above, the current decision or composer below, with native crease avoidance and a
scrollable interaction area. The view samples crease capability at creation; an image without fold geometry uses an
ordinary native Column/Scroll layout, without fabricated hover regions. Configuration changes do not replace the
view's input or transcript subtree. Ordinary posture uses measured interaction height, not a fixed 50/50 split.
A system photo editor returns a separate selected copy; system preview owns supported image zoom and document
viewing. The [HarmonyOS guide](../harmony/harmony.md) records the full interaction and verification contract.

The composer uses native GridRow/GridCol spans and order. Focus, draft, attachments, the actual composer width and
system font scale rearrange one TextArea in place; they do not replace its node or draft owner. At 1.5× or in a narrow
pane, fixed-size icon actions can stack without replacing the editor. A local font multiplier is not a substitute for the
native system scale. Verify focus and IME composition on device when width, keyboard or posture changes; static
layout and portable tests do not prove native composition continuity.

A HarmonyOS destination has one native route title and the Navigation-owned return action. Content does not repeat
that title or add another route-back button. Local actions such as returning to a file's parent directory or closing
a message thread keep their separate meaning. A missing resource title falls back to its identifier in the native
bar. Grouping uses native Select controls with visible values; secondary functional actions use compact native icon buttons.

Persona assistant question cards remain in the native transcript List alongside the stable composer. Each card keeps
its own request identity; ordinary sends leave withdrawal to the Hub, and non-blocking questions remain answerable.
Open editable questions begin with their prompt, without a generic answer subtitle. Pending, unknown and read-only
states stay explicit. Chat has no permanent Hub/computer/path subtitle. Agent permissions and plans keep their
existing approval gates.

Plan and receipt summaries open a native detail destination bound to the source conversation and item identity.
The detail locates that exact item from the feed head through successive cursors; a first-page miss is not evidence
that the item disappeared. Failed lookup can be retried. An inline cached plan summary may remain readable, but
plan writes need a fresh authoritative read and the existing owner/admission checks. Receipt detail distinguishes the Hub's `done`,
`failed` and `interrupted` states; `stopped` is derived only from a successful read showing the associated Todo is
still open, and a failed Todo read leaves the outcome unknown. Only `met` criteria count as passed; `criteria_more`
does not imply a result. Done/stopped receipts do not offer continue. Retry requires an eligible failed-start Todo
and current write capability; Continue requires an incomplete Todo, eligible lead Agent and current write capability.
Suppress a duplicate Continue while automatic recovery is active, and recheck recovery/reset state before admitting
an action.

Extension applications open from a root Navigation menu into a native destination and complete List, without adding
a peer view. Session actions share one native Menu with MenuItem rows of at least 48 vp from the title's More item or
the main Persona or hidden-title pane's More button. Read-only details and blocking decisions retain one fallback More button. These are the same captured actions, not parallel toolbars.

The file workspace uses native composed list items, three labeled system-icon controls and scrollable, automatically
sized local tabs. Computer transfer uses the system sheet, named computer switches, a native Radio list and Progress.
File names, machine names and complete paths wrap in scrolling content; they are confirmation data, not explanatory
captions. The target List hands boundary scrolling to its native parent Scroll. One footer uses native Flex wrapping
and minimum touch heights, with no second bottom inset. The composer and transcript scroll independently at large
text sizes; verification must reach each complete action within its own viewport. Theme primary fills include matching
readable foreground tokens in both modes. Reopening an active transfer retains its captured receiving computer and
folder; pending or unknown outcomes never silently resend.

Large system text never clips decision metadata: plan machine, path and permission wrap in native Flex. Inbox search moves above its native actions from 1.5×, with complete 48 vp icon targets. Preference labels and Select controls wrap when their combined intrinsic widths exceed the row. The native symbol peer segment keeps its shared 104 vp width and fixed-size symbols; content filters still use their native large-text Select fallback.

HarmonyOS functional actions reuse `IconActions`, `ButtonStyles.icon` and `iconPrimary`: native Circle controls with actual 48 vp bounds and 22 vp symbols, semantic resource colors, state-consistent accessible names and native pressed/disabled/focus behavior. Title tools may retain the existing 40 vp visual plus 48 vp response region. Content labels, selection scopes and system confirmation verbs stay visible. Unknown extension actions are grouped into named native menus, retaining their current fields, captured owner/page authority, explicit destructive role and independent danger tone. A caller cannot substitute a new page with the same action ID for an old menu callback.

This requested HarmonyOS presentation uses compact icon submissions while retaining the visible decision choices and execution context; it does not change Apple decision-row sizing. Approval/Deny use distinct symbols and colors, and pending/unknown never appears as an ordinary actionable Send. The sidebar shares the canvas; plan content has explicit context/goal/bounds/criteria hierarchy, question Answer is trailing, Todo rows have native group dividers, and card outer insets are applied once. Avoid repeated Details rows or descriptive subtitles. Recording, external sends, Forget Hub and extension installation retain visible consequences in a single native confirmation or permission sheet. The [HarmonyOS guide](../harmony/harmony.md#compact-native-actions) owns the complete interaction contract.

## 8. Inconsistencies and duplicate definitions

| ID | What | Where | Impact | Recommended unification |
| --- | --- | --- | --- | --- |
| DS-01 | Needs-you color: blue on Apple and Android (decided 11-11); amber on the web | `apple/Shared/StatusPalette.swift:20`; `spec/tokens/color.status.json`; `web/src/styles.css:233` | The web shows another color for the same state | Move the web to the same blue (its own issue) |
| DS-02 | Weights documented as bold; code is semibold | `docs/apple/ios.md` (heading, sectionHeader) vs `apple/Shared/Design/Theme.swift:338`, `:346` | Doc misleads new styles | Fix the doc |
| DS-03 | Docs list five radii; code has seven plus `appIcon` | `apple/Shared/Design/Theme.swift:181-198` | `thumbnail` and `page` undocumented | Document all, or fold `thumbnail` into `badge` |
| DS-04 | Duplicate hex values under different names (`#1D7A4C`/`#4CC38A` in Diff.add, Hue.green, unread text; `#2E6FD0`/`#5B9BFF` in loop and Hue.blue; `#B35400`/`#FF9F0A` in cautionText and Hue.orange; system blue literal written twice) | `apple/Shared/Design/Theme.swift:151,171,173,813-815`; `apple/Shared/StatusPalette.swift:24`; `apple/Shared/Design/Styles/Glass.swift:23`; `apple/Shared/Design/Palette.swift:8` | A change in one silently misses the others | Alias to one source; make the blue accent read `Palette.systemBlue` |
| DS-05 | Resolved 11-11 (#4209): Android's spacing has the iOS names and values, `spec/tokens` follows `Theme.Space` (checked by `scripts/assets/test_gen_tokens.py`), and the old 20 is gone: icons take `Tokens.Size.iconXS`, gaps the value iOS uses in the same place | `apple/Shared/Design/Theme.swift`; `spec/tokens/dimension.json` | None left | Keep the drift test green |
| DS-06 | Minimum target: web visuals 28 px. Android draws Small/Medium buttons 32/40 dp, chips 36, switches 26-30 and segments 35-36, and Compose extends their touch targets to 48 dp; the Glass and Board `JSegmented` frames clipped that extension away until they switched to `clipDrawing`; Android's switch, search clear button and heading actions reach 48dp through Compose's touch area without growing (`TouchTargetTest`, 11-11) | `web/src/ui/atoms.tsx:106`; `android/core/designsystem/src/main/kotlin/persona/core/designsystem/Controls.kt` (`clipDrawing`, `JSegmented`); `android/core/designsystem/src/test/kotlin/persona/core/designsystem/TouchTargetTest.kt` | Fails the baseline on web; Android meets it while no clipping parent cuts a target | Web: inflate hit area to the floor in the component. Android: frames around small controls clip drawing only (`clipDrawing`), and new small controls join `TouchTargetTest` |
| DS-07 | Two rounded-container primitives with different color sources | `apple/Persona/DesignSystem/Styling.swift:8` (style card color) vs `apple/Persona/DesignSystem/Components.swift:429-438` (system grouped color) | Clay, Island, Lark cards do not match neighboring surfaces | Make `.card()` read the style's card color; add an "inset well" component for `Fill.well` blocks |
| DS-08 | No shadow or elevation tokens; shadows are inline literals in six places; BeautifulUI elevation unused | section 2.6 | Inconsistent depth; bypasses the ratchet | Add `Theme.Shadow` from these literals |
| DS-09 | Off-grid magic numbers: card padding 14, chip padding 2.5, watch 10/12, toast and disabled opacity | `apple/Persona/DesignSystem/Components.swift:222,429`; `apple/Shared/Design/Theme.swift:431-451` | Cannot retune globally | Move to tokens; add an opacity scale |
| DS-10 | Overlapping size steps 1 pt apart; value-named boxes; 44 under six names | `apple/Shared/Design/Theme.swift:457-570` | Token choice is arbitrary | Merge neighbors; rename `boxNN` by meaning; alias the 44s to `tapTarget` |
| DS-11 | Motion: `spec/tokens/motion.json` holds the iOS shared set, checked against `Theme.Motion` and generated into Android `Tokens.Motion`. Android Glass's state and layout use its springs; Needs You breathes; composer send swaps/presses; swipe-to-reply uses `springBack`; the phone drawer uses `drawer`; chat unfolding and the closeout card fold use `reveal` (`CloseoutCard.swift:48`, #9897). `SharedMotion` exposes expand / reveal / conceal / drawer / springBack / keyboard; state/layout stay on `PersonaMotion` / `IosSpring`; composer on `ComposerMotion`. Open: wire `expand` / `conceal` / `keyboard` where iOS moves and Android still has no matching call site (Lark pin shelf, message-menu dismiss, composer open/focus); literal specs are ratcheted by `scripts/app/android_motion_baseline.py`; web has one curve | `apple/Shared/Design/Theme.swift`; `spec/tokens/motion.json`; `PersonaTheme.kt` (`SharedMotion`); `Unfolding.kt`; `CloseoutCard.kt`; `scripts/app/android_motion_baseline.py` | Remaining shared motions lack Android counterparts or still use local literals | Adopt each remaining motion where its iOS counterpart moves; keep the ratchet green |
| DS-12 | Resolved 11-11 (#4209): `spec/tokens/type.json` holds the nine `Theme.Font` levels with `Theme.Level`'s sizes and line heights, checked against the Swift, and Android Glass reads them | `spec/tokens/type.json`; `android/core/designsystem/src/main/kotlin/persona/core/designsystem/Typography.kt` | None left | Keep the drift test green |
| DS-13 | Radius philosophy: fixed on Apple, per-style on Android, literals on web (about 480) | `apple/Shared/Design/Theme.swift:178-180`; `android/core/designsystem/src/main/kotlin/persona/core/designsystem/PersonaTheme.kt:54-62`; `web/src/ui/atoms.tsx:973` | Same screen looks different | State the rule; replace web literals with tokens |
| DS-14 | Dynamic Type caps are literal sizes at 36 sites; only two are named | section 6.2 | Inconsistent behavior at large sizes | Name roles (glyph in button, badge, top bar) |
| DS-15 | Empty and loading states in several shapes: `EmptyStateView` vs `ContentUnavailableView` (22), `LoadableContent` (about 9 uses) vs 262 bare `ProgressView`, plain text | `apple/Persona/DesignSystem/Components.swift:385,739`; `apple/Persona/Views/SplitNavigation.swift:216` | Inconsistent look; BeautifulUI never applies to system views | Route empty and failed states through `EmptyStateView` with an error factory and Retry |
| DS-16 | Load errors in four shapes: `LoadProblem` (no Retry), inline red label in caption, label or secondary font, toast | `apple/Persona/DesignSystem/SettingsForm.swift:73`; `apple/Persona/Views/Knowledge/KnowledgeView.swift:84`; `apple/Persona/Recording/RecordingsSection.swift:40` | Different size, color and recoverability | One error component with optional Retry |
| DS-17 | Naming: `sm` (6) below `s` (8); font levels reuse Apple style names with shifted meanings; three suffix schemes in `Size` | `apple/Shared/Design/Theme.swift:206-209` | Easy to pick the wrong token | Document the mapping (section 2.2); consider renaming on a future token pass |
| DS-18 | Same control, different size by platform: shell switch 44 on Apple, 36 dp on Android; search field 44 vs web 32/36 | `apple/Shared/Design/Theme.swift:861`; `android/core/designsystem/src/main/kotlin/persona/core/designsystem/HomeTokens.kt:39`; `web/src/ui/SearchField.tsx:45` | Cross-platform parity | Align to the hit-area floor |
| DS-19 | Four composer implementations plus a page editor; send/stop styling and radius (32 vs 22 vs 20) differ | `apple/Persona/DesignSystem/ShellComposerBar.swift:11`; `apple/Persona/Views/Chat/ComposerParts.swift:7`; `apple/Persona/Views/Chat/PromptComposer.swift:11`; `apple/Persona/Views/Assistant/AssistantView.swift:1023` | Maintenance cost; dead branches on iOS | Make `ShellComposerBar` the only iOS composer; reduce the others to skins or delete |
| DS-20 | Bubble variants: padding 16 x 12 in chat vs 12 x 8 in Messages; gutters 48, 40, 32; strokes 0.08, 0.1, 0.22 | `apple/Persona/DesignSystem/QuoteViews.swift:305`; `apple/Persona/Views/Messages/MessagesViews.swift:420`; `apple/Persona/Views/Chat/MessageViews.swift:178,324` | Visual drift | One bubble component with tokens |
| DS-21 | Chip, filter-chip and button lookalikes hand-built in feature code (about 109 capsule backgrounds, 50+ view types); a second pale user-blue (`ChatBox.user` vs blue bubble) on both Apple and Android | `apple/Persona/Views/Inbox/InboxView.swift:770,1004`; `apple/Persona/Views/Inbox/InboxActions.swift:64`; `apple/Shared/Design/ChatLayoutTokens.swift:18` | Tint, padding and states diverge | Add an accent `Chip` look and a fill-width themed button; pick one user blue |
| DS-22 | Sheet presentation: detents `[.medium, .large]`, `[.fraction(0.6), .large]`, none; drag indicator on 18 of 43 detent sites; toolbar placement mixed | `apple/Persona/Views/Link/LinkSheet.swift:142`; `apple/Persona/Views/Chats/ChatsView.swift:348` | Unpredictable dismissal | A small `themedSheet` helper with a rule per sheet kind |
| DS-23 | Haptics: the mapping is written in section 4.3 (11-11, #4209), and Android's copy actions all follow it (`rememberCopy()`). Android's presses of send, stop, answers, the switcher and the persona button follow it too (`PressHapticsTest`). So do arriving requests (`ArrivingRequestsTest`) and archiving (11-12). Open: iOS's `Haptics` still lives in `AppModel.swift`, with one direct UIKit use | `apple/Persona/App/AppModel.swift`; `apple/Persona/Views/Chat/ChatEdgeScrub.swift`; `android/core/designsystem/src/main/kotlin/persona/core/designsystem/Clipboard.kt` | Android feels thinner than iOS outside copy | Adopt the 4.3 rows on Android screen by screen; move iOS `Haptics` to the design system |
| DS-24 | Seven confirmation mechanisms; destructive role used for non-delete actions; swipe-delete without confirm or Undo | `apple/Persona/Views/Todos/TodosView.swift:125`; `apple/Persona/Views/Mail/MailViews.swift:470`; `apple/Persona/App/AppModel.swift:3063` | Data loss risk; unclear meaning of red | Adopt the ladder in section 4.4; add Undo to archive |
| DS-25 | Resolved on Android 11-11 (#4209): one toast model as iOS's (`android/core/designsystem/src/main/kotlin/persona/core/designsystem/Toast.kt`: `FeedbackStyle`, one slot per window, iOS timing stretched by the accessibility timeout, announced, only its buttons take touches), drawn by the root and by windows of their own (the image viewer); pages' snackbars are gone. System toasts remain only where no Persona screen shows | `apple/Persona/DesignSystem/Toasts.swift`; `android/core/designsystem/src/main/kotlin/persona/core/designsystem/Toast.kt`; `android/app/src/main/java/persona/android/ui/Root.kt` | Web still differs | Move the web's toasts to the top capsule |
| DS-26 | Optional Messages page is a third primary destination against super rule 3 | `apple/Persona/Views/RootView.swift:182`; `apple/Persona/Views/Shell/ShellState.swift:23` | Rule ambiguity | Rule owner decides: opt-in extension page or amend the rule |
| DS-27 | Heading case: sidebar headings sentence case, Apple titles Title Case, with Title Case counted 1143 vs sentence 754 | `docs/protocol/i18n.md`, `docs/apple/sidebar.md` | Mixed catalog | Adopt the split in section 5.2 and normalize in one pass |
| DS-28 | Copy typography: apostrophes mixed, trailing ellipsis missing on 17 of 41 progress strings, "Retry" vs "Try Again", Stop vs End in recording | `apple/Shared/Localization/Localizable.xcstrings`; `apple/Persona/Recording/RecordingsSection.swift:161` | Polish | Apply section 5 |
| DS-29 | Localization defects: Health weight chart title uses the "Sleep" key; Mac microphone string says iPhone. The retired Persona Link prompt is no longer a live UI string | `apple/Persona/Views/Health/HealthTrends.swift:15`; `apple/project.yml:282` | User-visible errors | Fix the remaining strings (details in the audit) |
| DS-30 | Parallel token sets with silent drift: `ExtensionSurfaceTokens`, `IslandMetrics`, `WatchFaceTokens`, `Theme.Watch`, `WorkspaceSidebar`, `UsageColor` vs `Hue`, BeautifulUI spacing and radii | `apple/Shared/Design/ExtensionSurfaceTokens.swift`, `apple/Shared/Design/WatchFaceTokens.swift`, `apple/Shared/Design/Theme.swift:60-92` | Values copy `Theme.Space` without a link | Keep only what a target cannot compile; add a drift test for the rest |
| DS-31 | Web working and caution are one color: `--status-working` is `var(--orange)`, the caution role, so a direction-tree task's 在做 and its 卡住 / 待验收 (#4352) differ only by their word, where Apple has amber working and orange caution | `web/src/styles.css:264`; `web/src/lib/direction-tree.ts` (`DIRECTION_STATE_TONE`) | Working and stuck read as the same urgency at a glance | Give the web the Apple working amber as its own token (with DS-01's move of needs-you to blue), then point `--status-working` at it |
| DS-31 | Defined but unused tokens: `Space.lines`, `Space.block`, `Size.box140`, `Size.meter`, `Attach.badgeTarget`, `Syntax.all`, `Icon.forkChat`; on Android `Tokens.System`, `Surface`, `Shape`, `Watch` | `apple/Shared/Design/Theme.swift`; `apple/Shared/Design/Icons.swift:138` | Noise | Delete or use |
| DS-32 | `SecureField` still used for secrets on iOS; `HubTag` and `StatusLabel` dead | `apple/Persona/Views/Settings/ConnectorSignInViews.swift:553`; `apple/Persona/Views/Settings/ExtensionAuthViews.swift:206`; `apple/Persona/DesignSystem/HubComponents.swift:16`; `apple/Persona/DesignSystem/Components.swift:348` | Breaks a written rule; dead code | Use `SecretField`; delete the dead views |
| DS-33 | Pages read `AppStyle.current.usesBeautifulUI` and duplicate BeautifulUI forms (about 330 lines) | `apple/Persona/Views/Interactions/InteractionViews.swift:1323`; `apple/Persona/Views/Voice/VoiceBar.swift:135` | Two renderings to test | Move branches into design-system components |
| DS-34 | Reduce Motion not honored by navigation, toast, press and symbol effects | `apple/Persona/Views/Shell/ShellPager.swift:158`; `apple/Persona/DesignSystem/Toasts.swift:74`; `apple/Persona/DesignSystem/Styling.swift:187` | Accessibility | Gate through `Theme.Motion.reduced` |
| DS-35 | Resolved 11-11 (#4209): the Android ratchet scans the design system module too (331 color and 1193 dp literals at the start), so its literals can only go down | `scripts/app/android_token_baseline.py`; `android/core/designsystem/tools/token-baseline.json` | None left; moving the literals to tokens is phase 1's T2 to T4 | Keep lowering the baseline |

## 9. Recommended next steps

1. Fix the documentation and token sources that disagree (DS-01, DS-02, DS-03, DS-05, DS-12, DS-29). Cheap, high trust payoff. Regenerate Android tokens after correcting the JSON.
2. Close the accessibility gaps in the audit: hit areas, labels, Reduce Motion (DS-06, DS-34). Most of the P1 findings in `docs/design/HIG_AUDIT.md` are here.
3. Add the missing token families: shadow, opacity, and named Dynamic Type caps (DS-08, DS-09, DS-14).
4. Unify state components: empty, loading, error with Retry, and the confirmation ladder with Undo for archive (DS-15, DS-16, DS-24).
5. Consolidate duplicates: one composer, one bubble, accent chip and fill-width button, `.card()` on the style color (DS-07, DS-19, DS-20, DS-21).
6. Move haptics into the design system with a written mapping (DS-23).
7. Decide the cross-platform questions that need an owner: needs-you color on web, the third destination (DS-26), heading case (DS-27), per-style radii (DS-13).
8. Make drift visible: extend the Android ratchet to the design system, add a web literal ratchet, and a test that compares the Swift scales with the shared JSON (DS-30, DS-35).
9. Prune dead tokens and views, and replace the remaining `SecureField` uses (DS-31, DS-32).

## UI change self-check

Run this before opening a PR that adds or changes UI or an interaction, and paste the list into the PR with each item marked
done, not applicable, or deviated (with the reason). It is short on purpose. The sections cited hold the detail.

**Principles**

- [ ] Defaults first: works on a fresh install with untouched settings, and the default behavior fits in one plain sentence (principle 1).
- [ ] Progressive disclosure: advanced options are off by default and live in Settings or an advanced group, and the primary screens are unchanged when the feature is off (principle 2).
- [ ] User control: every new behavior-changing setting is visible, changeable and reversible. If Persona or an Agent can change it, the PR names the explanation, the confirmation and the rollback (principle 3).
- [ ] Consistency: lists the existing components, patterns and count definitions reused; a number shown under an existing name uses the same rule. Anything new has its reason written down and an update to this document (principle 4).
- [ ] Model and product separate: names which of speed, model, presentation or features it touches, and changes none of the others (principle 5).
- [ ] Checkable: each item above marked done carries its evidence (screenshot, test name, command output) in the PR (principle 6).
- [ ] Fast, local-first and stateful: local Sending is immediate, hub receipt shows Processing, failure recovers; before/after feedback and execution timings are separate (principle 7).
- [ ] Other platforms: done / issue #N / not applicable (reason)
- [ ] Parity list row updated

**Rules**

- [ ] Only tokens and design-system components: no literal sizes, colors, fonts, SF Symbol names, `glassEffect` or lookalike components. The token ratchets did not go up (sections 1.2, 1.3).
- [ ] Existing styles and theme colors look pixel-identical, except for a change that brings a screen in line with this document (section 1.2).
- [ ] Hit areas at least 44 x 44 pt (Android 48 dp); every control has a spoken label; gesture-only actions also have a visible control or an accessibility action (section 6.3).
- [ ] Destructive actions: confirmed if irreversible, offer Undo if reversible (section 4.4).
- [ ] Motion respects Reduce Motion; new loops are listed in `apple/PersonaTests/ContinuousMotionTests.swift` (sections 2.7, 6.3).
- [ ] Anything that opens or closes in a chat keeps the tapped row where it was, at the newest end and the oldest too (section 4.7.1).
- [ ] New copy follows section 5 and ships in `en`, `zh-Hans` and `ja`.
- [ ] Screenshots of each changed screen in light and dark, at the default and at the largest accessibility text size, are shown in the PR, not committed (iOS, plus watch, Android and web when touched).
- [ ] No new problem of a kind listed in `docs/design/HIG_AUDIT.md`. A PR that fixes a listed finding names its ID.
- [ ] Structure: in every grouping a session that needs the user is visible without opening anything; rows follow the work-item row; primary screens show no raw model IDs, session IDs, paths or engine names (sections 3.4, 5.6).
- [ ] Review size: says whether the change is large or small (section 1.4); a large one has had a person look at its screenshots.
- [ ] Performance (Apple clients): the review checklist in `docs/apple/performance.md` is pasted and marked; no new main-thread IO, expensive `@State` initial value or work in `body`, and a change to a hot path (chat list, composer, Agents list, pushes, launch, images) attaches before and after numbers.

## Page-sized settings and large-window keyboard shortcuts (Android)

- When `LocalLargeWindowInput` is true, `JSettingsPresentationFrame` presents the existing Settings navigation as a centered dialog card with the style's opaque surface, panel radius, one scrim and reduced-motion-aware fade; the enclosing `JSheet` clears its full-window container and outer scrim so they do not blend twice. Compact and wide-phone windows keep the original opaque full-window presentation. `SettingsPageSizing.maximumWidth` and `maximumHeight` alias `ReadingWidth`; `minimumWindowMargin` is `Tokens.Space.xl`. The card is centered inside `safeDrawingPadding`; it does not read a device model or introduce another window breakpoint. The existing list-detail directive still decides when its two panes fit and falls back to one pane without squeezing them.
- Presentation changes keep the one `settingsSheetStack`, one `JSheet`, one presentation frame and one content invocation. Only the container and card geometry change, preserving non-saveable state and running effects as well as the fixed-key `SaveableStateHolder`; they never open Settings again, move content between dialog compositions or key a stack by window size. `focusRestorer` returns to the previously focused field only after a presentation transfer and only when no field already has focus. The outer blank-tap callback is an explicit user clear and disables restoration rather than reopening the IME on the next resize. Root Close dismisses the whole card and child Back pops one Settings destination. Book and tabletop use the same safe area and existing internal pane policy.
- `HardwareShortcutHost` keeps the same parent and content call site in every window and provides its controller and `onKeyEvent` listener only in a large window. On entering a large window the shell host uses public focus state to request its own focus only when that window has none; Settings disables host auto-focus so dialog content keeps its field focus, and an already-large resize makes no focus request. Its listeners run after focused children, never in the preview phase. Shared text inputs, including `LargeWindowComposer`'s small field outside IME composition, handle only Ctrl+/ in their own preview phase because editors consume it before a parent listener; all other keys follow the ordinary path, so a focused terminal or IME keeps Escape, Tab, Shift+Tab, Ctrl+C, arrows and Ctrl+/. `JSheet`, `JDialog`, `FullTextReader`, `MessageSharePage` and `ChatSearchScreen` register their existing dismiss callbacks with stable tokens; Escape invokes only the last registration and consumes its key-up and repeats. With no registration it is a consumed no-op, never App exit, navigation pop or Agent interruption. `ExpandedComposer` and `JPointerMenu` retain their own Escape handling.
- Ctrl+/ opens `KeyboardShortcutsSheet` as the top dialog. It lists only implemented send, newline, dismiss, help and existing terminal keys, with `en`, `zh-Hans` and `ja` resources and a single-line Close button. Its content scrolls at large text sizes. Opening it does not clear a draft, change a selected conversation or stop terminal work; closing the dialog returns to the previous window and its focused content. Compact windows add no help entry or new key handling.

## Configuration forms (web, #9877)

A module's settings described as a configuration Document (`docs/web/web.md`, "Configuration forms") are drawn by one
generic form, never a page of their own: the document's title as a group heading (`text-[12px]` `ink-3`) with its
description under it, then each group as a settings `Group` (its title above, its description as the footer). A field
is a row: the title in the row's label type, its description in the hint type, the control, any error adjacent under it,
any notice (orange for a warning, `ink-3` for info), the disabled reason or "Read only", and the module's details as a
two-column definition list in `text-[11.5px]` (`ink-3` terms, `ink-2` values, the page's existing `grid-cols-[auto_1fr]`).
Every field ends in one footer row: the default value, the scalar count where a text field has a maximum, the polite save
state (**Saving…** with a spinner, then a green **Saved**) and the quiet **Reset** / **Cancel** / **Save** actions that
apply, so the actions never sit between the control and its explanation and a short default keeps them on one line.
The footer reserves the small-button height when no action is shown, and longer content can still wrap.
- Controls, all existing: a toggle is the `Switch` at the row's end; a select is the `Segmented` control at the end
  (`segmented`), a native select in the settings field style at the end (`dropdown`) or `ChoiceRows` under the text
  (`list`, the default); a multiselect is a list of `role="checkbox"` rows with `CheckMark`; text is the field style's
  input or textarea, a number a narrower input with a decimal keyboard. Text and numbers are drafts, and a multiselect's
  checks are the same kind of pending edit, until **Save**; a multiselect whose maximum is one is drawn as a single choice
  (`role="radio"` rows with round marks and arrow keys), with exactly one Tab stop — the chosen option, or the first when
  none is chosen (no radio is marked chosen) — and choosing another option replaces the old one in one click (the wire
  value stays an array), while a larger maximum keeps ordinary checks and can pass through a temporary count. **Reset**
  appears where the value differs from the default.
- Whole-form lines (a version it can't write, a conflict or an uncertain write with **Reload**, a refresh that failed with
  **Try again**) are one card above the groups.
- An unknown kind or invalid definition keeps its title, description and details, with an orange line instead of a
  control. A known field whose saved value is missing or unreadable shows an empty input or unselected choices, including
  an On/Off choice for an unreadable switch. The hint is part of the control's accessible description. Choosing or
  entering a valid value repairs the field, and **Reset** returns the default.
Why one form: the user asked for settings that modules inject with one portable contract and shared styles, so a new
module adds data, not a page, and the same states (pending, saved, failed, conflict, read only) look the same everywhere.

**Labs** (`docs/web/web.md`, "Labs") is a sidebar group of its own without a title, with the existing `sparkles` glyph (a
dedicated one would not fit the first screen's budget); its page is an introduction card with a note under it that each
item states its own scope, then one configuration form per module. Where a feature also has its own options menu
(Terminal settings), it shows the current value read-only with a button that opens the same Labs page in a `Dialog` over
the page (never navigating away, so nothing open behind it, such as a chat's unsent draft, is unmounted) instead of a
second switch, so there is one place to change it.

An experiment can supply a permanent note under its introduction or beside a field, using the existing info and warning
hint styles. A choice with consequences can request confirmation for a specific change. Use the shared Dialog, with the
affected scope, impact, timing and an action-specific button. All dismissal paths preserve the edit. On success the
field’s polite save status can explain when the choice takes effect. Show this receipt once, in the field footer.

## Extension entry lists

A declared list of extension accounts, gateways or similar entries (#9504) belongs in the existing **Settings** group; it does not create a page, tabs or an extension-owned layout. The audience is an everyday owner managing a small list, so rows use the entry's name and plain state words, never its host-owned ID, array index, secret name or engine terminology. Reuse settings `Group`, `Row`, `Button`, `IconButton` and `Dialog`, the existing radius, surface, hairline, spacing and type roles; do not introduce a second card style or new tokens for one extension.

- Each entry is one surface with a title and secret state, then its declared flat text fields. Prefer its `name`, then its first nonempty field, then a localized numbered title. **Secret** opens the shared write-only secret dialog and names both entry and secret. Before the entry is saved, the button is disabled and the reason says to save first. Entry secrets never also appear in the fixed Secrets group.
- **Add entry** appends one empty entry and is disabled at the manifest's maximum. All field edits and additions remain drafts until the page's existing **Save**. The host owns saved entry identity; an unsaved browser key must survive immutable field edits without becoming a visible value or part of the JSON payload.
- Removal uses the shared confirmation `Dialog`, names the entry, and says that the entry and its saved secret are deleted **when Save succeeds**. Cancel leaves the draft untouched. A required list keeps only its declared minimum; an optional list can always return to its valid absent state even when a present array would have a positive minimum. If an address participates in configured-network permission, a nearby hint says that saving a changed address clears that entry's secret. These are consequences of the pending configuration write, not claims that merely opening or dismissing the dialog changed hub state.
- Text and explanations wrap at phone width and the largest accessibility text size; buttons stay on one line using the existing button behavior. Destructive actions remain reachable without horizontal scrolling. Dark and light themes use the same semantic roles as every other settings card.
## Sidebar icon colors (#10822)

Who it serves: anyone whose sidebar lists many pages and who finds it quicker to scan when each page has a color. It is
an option, off by default (principle 2): Settings › General › Appearance › **Sidebar icons**, **Monochrome** or
**Colored**, kept in this browser as the theme color is (`web/src/lib/sidebar-icons.ts`). Monochrome is the sidebar as
it was, pixel for pixel: every page's line icon in `--ink-3`, the open page's in `--ink`, and nothing is stored.

With Colored:

- Each built-in page's icon takes the fixed named color below (`Color`, `schema/extension/view.schema.json`), drawn in
  the web's existing `--hue-*` tokens through the same `hue()` a row's `icon_color` uses
  (`web/src/lib/extension-ui-v6.ts`). No color value is added.
- An extension's `nav_page` takes the `icon_color` its manifest names (`docs/extensions/extensions.md` "UI slots"); a
  page without one, or with a name no client knows, stays neutral.
- The open page keeps today's rule: its icon is the open row's `--ink`, not its color, so the open page stands out as
  before. Under the pointer the color stays and the hover fill shows.
- Everywhere the sidebar draws its pages: the Pages list and the groups, More, the folded icon rail and a phone's grid
  of pages. The More menu, ⌘K and Settings › Sidebar pages keep neutral icons; the main chat's row, All chats and the
  Agents' rows are not pages and are unchanged. State marks, dots and counts never take these colors (#4538).
- Theme colors and extension themes move neither the `--hue-*` tokens nor the sidebar's backgrounds, so the colors and
  their contrast are the same under each of them; the text size does not scale icons.

| Page | Icon | Color | Light: on `--canvas` / `--hover` | Dark: on `--canvas` / `--hover` |
| --- | --- | --- | --- | --- |
| Overview | `home` | amber | 3.26 / 3.35 | 9.05 / 7.59 |
| Inbox | `inbox` | orange | 3.01 / 3.09 | 7.16 / 6.01 |
| Health | `heart` | pink | 3.82 / 3.93 | 6.44 / 5.41 |
| Messages | `chat` | green | 3.25 / 3.34 | 6.87 / 5.76 |
| Workflow board | `layers` | yellow | 3.25 / 3.34 | 10.89 / 9.14 |
| To-dos | `todo` | blue | 3.23 / 3.32 | 5.84 / 4.90 |
| Scheduled tasks | `clock` | purple | 4.25 / 4.37 | 6.53 / 5.48 |
| Knowledge | `book` | indigo | 4.79 / 4.92 | 5.80 / 4.87 |
| Machines | `machine` | teal | 3.65 / 3.75 | 7.15 / 6.00 |
| Settings | `gear` | gray | 3.54 / 3.63 | 6.27 / 5.26 |

Each page has a color of its own. Red (3.55 / 3.65 light, 5.09 / 4.27 dark) is left to extensions: on a built-in page
it would read as an error. An icon is a graphic, so the floor is 3:1; `web/src/ui/theme-contrast.test.ts` checks all
eleven named colors on both backgrounds in light and dark, since an extension may name any of them, and the closest is
light orange at 3.01:1. Why a new pattern: one ink let only the open page stand out; Colored keeps that and adds a
second cue for scanning, so it is an option and never the default. iOS and Android follow with the same setting, the
same map and the same `icon_color`, so a page has one color on every client (#10822 tracks them).

## A chosen page that is not there now (#9071)

A setting that names something which can come and go (the web's **Open on**, Settings › General › Start page: a page of
the sidebar, an extension's included) keeps the user's choice when its target goes away, and the product uses the default
meanwhile, without a toast or a dialog at the moment it happens. The setting itself says so: its menu keeps the choice
selected under a plain name (**A page that isn't available now**, not its id), and the row's line under the label, the
`Row` hint in `--ink-3`, changes from what the setting does to why it falls back and until when (**…opens on the main chat
until it's back**). Choosing anything else in the same menu replaces it. Reason: the target is usually away for a while
(an extension turned off, another default assistant), so clearing the choice would undo the user's decision behind their
back (principle 3), and a toast at every opening would be noise for something they can read where they would change it.
The same rule as a chosen look whose extension is switched off ("the choice stays: the default only shows meanwhile",
`server/internal/hub/looks.go` `looksGone`). It reuses Settings' `Row`, its hint line and a native select; no new component.

## Arranging cards in place (#9745)

Overview's extension cards (web) are arranged where they are drawn, not in a settings list: the result is a layout (which
card is wide, which sit side by side), and a list would show neither widths nor neighbours. Overview's top bar has
**Arrange Overview** (`IconButton` with the `grid` icon, `aria-pressed` while on) beside **New agent**, always: Overview's
own sections are arranged with the cards (#10905). The edit state reuses the sidebar pages editor's pieces (`web/src/views/settings/SidebarPages.tsx`): the `grip`
handle and an HTML drag onto another item, which takes its place; ↑ / ↓ `IconButton`s at 26 px for the keyboard and touch;
a native select for the width (**Half row** / **Full row**); `Switch` for shown or hidden; the hub footer's words for where
it is kept; and **Reset** with Undo in a toast (4.4). Each card keeps its surface and its content, made `inert`; its head
becomes the controls, which wrap under the name on a narrow card. A hidden card is drawn only while arranging, as its
hairline outline (`shadow-hairline`, no surface) with "Hidden: not loaded" in `--ink-3` under its name; the card a dragged
one would land on has a 2 px inset `--accent` ring. No new token or component (`web/src/views/extensions/OverviewArrange.tsx`).

Overview's own sections (#10905) take part in the same edit state, in their places among the cards: each is drawn as a
bar the row's width, on the card surface (`rounded-card`, `bg-surface`, `shadow-card`), with the same `grip` handle and
drag, its name as its heading would be, and the same ↑ / ↓ and `Switch`; no width select (a section always takes the
row). Its rows are not drawn while arranging: a section can be long (every computer, every working agent), and its place
is what is being chosen; a section with no rows now gets its bar all the same, so it can be placed before it fills. A
section that holds what needs the user (Needs you, Stuck) has no switch and says "Always shown" under its name in
`--ink-3` (3.4: a session that needs the user is never hidden); a hidden one is the same hairline outline as a hidden
card, "Hidden" under its name. The greeting and its summary are not part of it and stay first. No new token or component.

## Arranging conversations in place (#12017)

The Assistant's ordinary conversations use the same progressive-disclosure pattern in the sidebar's Recent and in All
chats. **Arrange chats** lives in an existing row/display menu, so an untouched list has no extra handles or buttons.
While arranging, each row keeps its own content, made `inert`; a grip starts an HTML drag onto another row, and named
**Move up / Move down** `IconButton`s provide the same action to keyboard and touch users. The edit heading explains
that the order is shared, with **Use activity time** (Reset with Undo) and **Done**. The main conversation and the
Directions group stay fixed; a new or not-yet-loaded conversation remains visible in a deterministic place rather than
being discarded. Android uses the same edit state and actions with its native long-press drag pattern. With no saved
choice, the existing activity-time groups and rows are exactly as before. This is presentation only: it neither pins a
conversation nor changes supervision. The shared row and edit heading add no color, spacing or elevation tokens.

## Choosing conversation tag filters (#12018)

All chats keeps its automatic row of one chip per tag in use until the user chooses **Customize tag filters…** in the existing display-options menu. Editing happens above the same list, not in a detached settings page: an exact-tag field and **Add filter**, a small × beside each chosen chip, **Reset to all in-use tags**, and **Done**. Chips still filter the list by exact tag while editing. Removing the selected chip also clears that filter rather than leaving an invisible selection; Reset restores the automatic chips. A deliberately empty custom row says **No tag filters** instead of silently refilling itself. Saves are optimistic and a refusal restores the previous entries and reports the error. The hub stores the exact entries and their order for every client; null means automatic and an empty array remains a real choice.

The filter bar is a controlled component: its caller owns the entries, order, selection and editing state, and the bar owns only its draft and pending action. It can therefore move into another pane without moving conversation-list state with it. Default chips have no persistent editing controls, a fresh install with no tags has no empty row, and no sidebar structure, navigation pattern, token or dialog is added.

## Values a form fills in by itself (#4975)

Where a form can work out a value from what the user already gave it (the web provider wizard's Claude Code tiers, read
from the checked models), it fills the value in and marks it with a neutral **Auto** `Pill`, in the summary and on the
field, with one line under them saying that automatic values follow what they were read from until the user changes
one. A value the user picks is never filled in again, an empty one ("Same as default model") included. When what it was
read from changes (a model unchecked, another default), only values still marked Auto follow, and one that no longer
fits goes back to empty. Something already saved is never refilled when it is opened: when what it holds would read
differently now, one line under the summary says what it suggests and **Apply** (the status line's action) takes it.
The line warns (`--orange` with the warning mark) when the current value costs more for no reason, such as light
requests running on the heaviest model while a lighter one is checked, and otherwise suggests (`--ink-2`). The summary
is the inset well (`rounded-card`, `bg-inset`, `shadow-hairline`) with label / value rows; **Adjust** is an accent text
button with `aria-expanded` that swaps the rows for their fields in place, and rarely changed fields stay under
Advanced (principle 2). Reason: a default the product can work out should not wait behind a folded group (principle
1), and an automatic value the user cannot tell from their own, or one that changes after they set it, would take
control away from them (principle 3). This reuses the wizard's field, pill and status-line styles and adds no setting,
style or motion.

A value the product measures rather than reads follows the same rule with a line instead of a pill: the wizard's sign-in
method (#5329). On **Automatic**, the default under **More options**, the Connect step's own check finds whether the
gateway takes Bearer or x-api-key, and one quiet line in the check box names it (`--ink-2`, no icon, after the test
result and before the model count), with **Change** (the status line's action) opening More options and focusing the
choice; the `Segmented` there offers Automatic, Bearer and x-api-key. A saved provider's line names its stored method the
same way. Before the check has found anything the line is absent rather than a guess. Reason: a question with a right
answer the product can measure should not be asked (principle 1), and the line keeps the answer visible and one tap from
changing (principle 3).

## Codex speed controls (#3865)

An Agent's Codex thread sets its speed with Fast in the model panel's flyout of the model it runs (#5032, the model pill
and unified model panel in 3.8); a Persona chat running Codex does the same since #5033, its effort chip and Speed
submenu retired. Fast is a toggle showing the current value, and its label explains the increased usage. A change
waits for native acknowledgment and the session push; turning Fast off is the reversible choice. Settings > Agents >
Codex uses the existing agent-settings row for the native default. No account or session is automatically switched to Fast.

The iOS main Assistant conversation offers the same current-session choice inside its existing composer Model / Thinking
Effort menu (#10244), not in Settings and not on other chats. A native **Speed** section is present only when the session
advertises the known tiers; **Standard** and **Fast** are localized plain labels rather than native display metadata.
Fast's detail says **Uses more quota**, and a separate row says **Applies next turn**. Working leaves the choices
available; an offline or read-only action is disabled beside a one-line reason, and an outstanding change says **Changing Speed…**. A pick sends only `service_tier` in its own
request and never changes model or effort. The checked value is only the hub-acknowledged `service_tier`, never an
optimistic pick or an inferred Standard; summary updates preserve it and the advertised tiers. Refusal and timeout show a
visible error and leave the last acknowledged checkmark. The section reuses native menu switches and adds no token,
component, style or navigation pattern.

A new Persona conversation on the web reuses that same model flyout (#6240), inside Agents-facing customization rather
than adding an everyday-facing setting. While the default manager is in use, the hub feature
`assistant_start_service_tier` comes from that manager's hello, and a selected model whose native `service_tiers`
advertise `priority` has its Fast toggle available before the first message; other models, managers and older hubs do not
show it. The local Fast choice and reasoning effort are independent (principle 5), enabling Fast pins the displayed
launcher, model and effort, and an untouched start preserves the native default (principle 1). The first request carries
only an explicit choice, which must be natively acknowledged before the first turn; a refusal is recoverable and never
silently means Standard (principles 3 and 7). A successful start, Follow default, a dropped pick or a manager change
clears the one-off draft; a refused start retains it. New Agent has no start speed control.

## Hub connection password (#4474)

The administrator's Devices page owns the connection-password switch and registration form. It is off until an
administrator sets a password and enables it. The form uses the existing admin Card, Button, ErrorBox and toast
patterns, and explicitly offers revocation of existing device credentials. Disabling password sign-in or replacing
the password is visible here; neither action silently removes devices unless the administrator selects revocation.

The web sign-in page shows the password form only when the hub enables it, alongside the existing pairing entry.
Native pairing offers password sign-in with a hub address and keeps scan/link pairing reachable. The system browser
owns password entry so all clients use the same OPAQUE implementation; the callback carries a single-use code bound
to the initiating app rather than a reusable device token. Password sign-in requires trusted HTTPS, with QR pairing
remaining available for self-signed deployments. This changes connection features only, without selecting a model,
speed, persona, style or theme color. Android reuses PairingHeading, JCard, JTextField, JButton and SettingsCaption;
Apple reuses the pairing Form, Section, Picker, toolbar and Theme tokens.

## Meeting page (#3862)

### The floating meeting window (#8731 V5)

While a meeting records and its room is put away (the room's collapse, or swiped down), a small card floats over every
page of the app (`MeetingFloatingWindow`, on `RootView`). The room's full-screen cover lies over it, and it goes with the
recording.

- **Look:** dark in both appearances, as the room is. The first line has the recording's dot (gray while paused), the
  meeting's name and the time in the danger color. The second line has who speaks now: their face (a green ring while they
  speak; Persona's avatar with the purple ring), the word for it and a small waveform that moves while someone speaks.
- **Who it can tell** (E7, `MeetingFloatSpeaker`):
  - **Persona is answering** while the phone says its answer or the meeting call's voice speaks;
  - **Someone is speaking** while the microphone hears a voice (a gate that opens at one level and closes after a hold
    below another, so a breath doesn't flicker);
  - **You're speaking** instead when the microphone is a headset's, which hears little but its wearer;
  - **Quiet** otherwise, **Paused** while paused.
  - No numbered speakers: the phone can't tell people apart.
- **Interaction:**
  - a tap brings the room back (`MeetingRecorder.openRecordings`);
  - dragged, it settles on the nearer side at that height, inside a band clear of the top bar and the bottom's composer
    and tabs (`MeetingFloatPlace`);
  - pulled past a side, it docks there as a bar with the dot and the recording mark, and the bar's tap brings the card
    back. VoiceOver has Move to the Side.
  - No buttons in it: Pause and End are the room's and the Dynamic Island's.
- **Tokens:** `Theme.Size.meetingFloat*` and `Theme.Recording.float*`.

### After the meeting (#8731 V3)

A meeting that ended is one screen: `MeetingPage`'s after mode. It opens after End in the meeting room, and from the
Meetings list and Knowledge for every meeting recording (`RecordingOpen`). Notes and all-day recordings keep their
recording page.

- **The bar:** in the sheet its ✕ at the leading corner (`SheetClose`, #9653), pushed its back button. Trailing, the ⋯
  menu: Rename and **Recording Details**, the recording's page (`RecordingDetailView`, with its audio, sharing,
  regenerating and deleting). That page shows no Talk about this meeting, since it was opened from here.
- **From the top:**
  - the name, then when it ran, how long, who was in it (you, and Persona if it answered at any point) and Cloud
    transcription when the hub transcribed it (`MeetingAfterHeader`);
  - the **summary card** (`MeetingSummaryCard`): the minutes' headline and at most three core conclusions, or the first
    two points of older minutes, then three counts: decisions, to-dos and open questions (`MeetingAfterSummary`).
    While the minutes are on their way the card holds where they are (Writing the minutes…), and the summary comes in
    its place;
  - the **to-dos**: the minutes' own Action Items card, with Add to To-Dos and Add All (`MinutesCard`);
  - the **meeting map** (`MeetingMapCard`): the minutes' topics with their bullets, else the topics the room kept during
    the meeting, else no map (`MeetingAfterMap`). Its first five topics are drawn as the meeting room's Map page draws
    them (`MeetingTopicMapRows`), and **Full Screen** opens that page (`MeetingTopicMap`);
  - the parts by name (`MeetingTabRow`, as the meeting room's): **Minutes** (the full minutes, as on the recording's
    page, each card boxed: `minutesBoxed`), **Transcript**, **Photos** (a room meeting that took photos) and
    **Conversation** (the room's thread and the follow-up). Conversation shows wherever the room or a follow-up has one,
    since the composer writes there.
- **The composer:** "Talk with Persona about this meeting" stays in the bar. Typing opens Conversation, which follows its
  newest end as the thread did; the other parts open at the top.

### The meeting room (#8731 v2)

A room meeting records in a meeting app's room: full screen and dark in both appearances (E1, as meeting apps are; the
wrap-up after Stop takes the app's own look again). From the top: collapse (`Theme.Icon.minimizeRoom`; the meeting goes
on, the Live Activity and the page's card lead back), the title over its time and state, the page menu; who is in it
(`MeetingRoomPeople`: you, with a green ring while the room records and Persona is quiet; Persona while it answers, with how
it answers and the green ring while it speaks; Invite while it isn't in, and the bar's Remove takes it out); then, for a
meeting started with the live view, three pages that swipe and are named in a row of chips (`MeetingRoomTabs`): **Live**
(the newest lines and the thread, as before),
**Highlights** (the topics and the highlights card) and **Map** (`MeetingTopicMap`: the topics in order, the one on now
in the positive color, those to come dimmed, each with its note). The bar keeps the question field and Talk; under it a
row of round controls (`MeetingRoomButton`): Pause, Bookmark, Photo (a room meeting with photos), **Speaker** (Persona
answers out loud or on screen; only while it is in), **Invite / Remove**, and **End** in the danger color, which asks
first, in a question anchored on it (End and Write the Minutes / Keep Going). The system's echo cancellation stays as
the meeting started, unless Settings › Meetings' **Experimental** switch, **Echo Cancellation with the Speaker** (off by
default, B2), is on: then the speaker turned on mid-meeting turns it on to the end, and the microphone stops for a
moment. Ordinary recordings and #3746 meetings keep the live view, in the same dark full screen.

The live recording sheet's page for a room meeting (a phone meeting recording started while the hub listed
`meeting_conversation`; contract: `docs/assistant/meeting-minutes.md`, "Meeting conversation (#3862)"; the phone's side:
`docs/apple/recording.md`, "The meeting conversation (#3862)"). It is `MeetingPage`, shown by `RecordingLiveSheet`: live
while the meeting records, and after Stop the same page in its after mode, continuing as the meeting's thread; any
finished recording's page with something to talk about (a note, a meeting no room attended, a room meeting) pushes the
after mode (row **Talk about this meeting**, phase 6). Ordinary recordings and #3746 meetings keep the live view and the
wrap-up unchanged while they record. Reason: the meeting is one conversation from its start to its follow-up, so the
page the user watches during the meeting is the page they keep writing in after it.

- **Structure, live**: a header (**Hide** first, the room's ⌄ at the top left (#9653), the title in `Theme.Font.title`, the page menu), a state line (the recording dot,
  the system-ticked clock, the state word), a subtitle (the role, "On this iPhone", "Private" when marked), a problem as a
  caution `NoticeBanner` when the room can't take part, **Topics** (phase 2, below), the top card (**Highlights** in
  phase 2b, below, or else the **Live** box), the thread's section header, the thread, and the bar pinned through
  `safeAreaInset`. While the field has the keyboard the top of the page steps aside as the bar's controls do: the
  subtitle and Topics hide and the top card keeps only its newest part (the Live box its newest line; the Highlights
  card its newest highlight, the latest `at`, or its waiting line; neither with Full Transcript), so the thread keeps its
  height (the structure stays the same, so the thread's scroll position holds); it changes on `Theme.Motion.state`, and
  when the keyboard goes down the card is whole again. In the meeting room (#8731 B1) the people row and the tabs step
  aside instead, and only the Live page folds (its Live box to its newest line). The Highlights page keeps its card
  whole, Full Transcript included, since the card is that page. At accessibility sizes a one-line header (dot, time, menu, Hide)
  stays fixed, the rest scrolls as one, nothing folds, and the bar stays pinned.
- **Structure, after**: since #8731 V3, "After the meeting" above: the facts, summary, to-dos and map over the parts by
  name, the Conversation part holding the room's thread, the **After the Meeting** divider and the follow-up's thread, and
  the composer alone in the bar. While the keyboard is up the screen is the conversation under one fixed line (the minutes' state, Open as an icon with its name as the label), and the
  title scrolls with the thread. While the hub would still answer `409 meeting_running` the composer is disabled with a
  caption that says why: the end still on its way ("in a moment"), the end refused or the sync paused (the sync's own
  problem; the hub ends such a meeting 12 hours after its last stored upload), or an older manager without the follow-up.
  When pushed it has the navigation bar's back and no Open button (the recording's page is behind it).
- **One composer, #4486's field**: the in-meeting typed question's `MeetingQuestionField` (section 4, "In-meeting typed
  questions (#4486)"): the field (`ComposerSurface`, a vertical `TextField` that grows to `Theme.Recording.askLines`
  lines, `askLinesLargeType` at accessibility sizes) and the round send button (`ComposerRoundButton`, dimmed with
  `Theme.Recording.askSendIdle` while there is nothing to send), with a `[+]` menu before them built as
  `ShellComposerBar`'s, with its plain [+] face (the row's one solid button is send). Return sends; the keyboard goes down
  after a send. It writes to the room (an ask), to Just me, or after the meeting to the follow-up. Words over the
  contract's 8000 characters (Just me, and the follow-up's first message, which goes through `…/followup`) keep Send off
  with a caption saying so; nothing is cut. Why not `ShellComposerBar` itself: its dictation button would take the
  microphone the recording holds, and its model and attachment rows don't apply. The page has no second composer.
- **Control row** (live only): Pause / Continue, Bookmark and Stop, with the live bar's looks, identifiers, Stop
  confirmation and announcements; hidden while the field has focus. At accessibility sizes the bar is at most two rows,
  the field and one row of icons: Pause or Continue, Stop, a More menu (Bookmark, the Just Me switch, the photo items
  when the meeting takes photos, **What Was Just Said**) and send. Why the field takes no words then shows at the end of
  what scrolls (a neutral `NoticeBanner`) and as the field's spoken hint, not in the bar.
- **When Persona can't take part**: an older manager took the room meeting with Persona off (the last upload's `200` had
  no `room`): the subtitle and the thread's header say "Persona can't take part right now", the field asks nothing of the
  room and [+] goes, with a caption that says so; Just me stays. The same rule (`MeetingAsk.canAsk`) decides the asks
  themselves and the Live Activity.
- **Topics (phase 2)**: in a meeting started while the hub listed `meeting_live_view`, once the room set any, a section
  under the header and above the Live box: "Topics" (caption strong, secondary, a header for VoiceOver) and one
  horizontal row of `Chip`s in the room's order, scrolled so the current one is in view (without motion under Reduce
  Motion), each at most `Theme.Size.meetingTopicChip` wide (scaled with the text; a longer title is cut). The state is in
  words, never the color alone: **Done: <title>** on the neutral fill, **Now: <title>** in the accent tint, an open topic
  its title alone in the outline look (the sketch's `[ ]`); VoiceOver reads each as one element with its state, an open
  one "Not yet: <title>". It steps aside while the field has the keyboard, as the subtitle does; at accessibility sizes
  it scrolls with the rest. Nothing shows before the first topics. Reason: the topics say where the meeting is at a
  glance, and words keep them readable in either appearance and to VoiceOver.
- **Live box**: a `.surface` card with the radius `Theme.Radius.card`: "Live" and, on the right, how far the transcript
  is behind ("About N s behind", or "Listening…" before any line), then the last `Theme.Recording.liveLines` sentences
  (time in monospaced digits, secondary; two lines of text at most) and **Full Transcript**, which pushes the whole
  timeline. It is the top card only where there can be no highlights: a meeting started on a hub without
  `meeting_highlights`, or one that has none while Persona doesn't attend (the Highlights card's rule, below).
- **Highlights card (phase 2b)**: in a meeting started while the hub listed `meeting_highlights`, the top card in the
  Live box's place, the same `.surface` card, the gist of the meeting so far in the room's words (they may differ from
  what was said): "Highlights" (caption strong, secondary, a header for VoiceOver), then each highlight, earliest first,
  all of them (at most four): its time from the recording's start as the minutes cards' time (`RecordingFormat.clock`
  in a compact accent `Chip` inside a 44 pt hit area, read as "Show at m:ss in the transcript") and its words in
  `Theme.Font.body`, at most `Theme.Recording.highlightLines` (2) lines at the regular sizes, cut at the end (VoiceOver
  reads them whole); the words are not a button. At the accessibility sizes the words take what they need and the time
  goes under them. The last row has **Full Transcript** at the trailing side. The card shows no transcript lines: during
  the meeting the owner reads the points, not the words as said, which are one tap away. Before the room's first
  highlights, while Persona attends and its room still gets the meeting (the role isn't off and the hub took it, the
  phone still sends the meeting's lines, and the room isn't unavailable, nor missing on an older manager: never under a
  banner saying Persona can't take part), the card holds one waiting line in their place, "Persona is listening.
  Highlights come after a few minutes." (`Theme.Font.secondary`, secondary), above Full Transcript; it cross-fades to the first highlights in place, one over the other, on `Theme.Motion.state`
  (at once with Reduce Motion). Highlights already shown stay after the role turns off; a meeting with none by then
  shows the Live box, cross-fading in place the same way. A time pushes the whole timeline scrolled to the first of the
  phone's lines in that highlight's minute (`at` to `end`), every such line lit for two seconds with `Theme.accentTint`
  (cleared on `Theme.Motion.standard`) and read by VoiceOver as selected meanwhile, or the line
  `RecordingMinutes.sentenceIndex` picks when none starts there. Nothing animates as the page opens; a new or changed
  highlight is never announced. Reason: the owner sees what the meeting has come to at a glance, and the words as said
  stay one tap away on the Full Transcript page.
- **The thread**: rows read as one element each. The room's first message is a quiet centered line ("Persona joined the
  meeting"; none when the meeting began with Persona as the note-taker), and so is Persona invited or taken out while
  it records ("Persona joined / left the meeting", where it happened); each batch of live lines is a collapsed capsule ("N lines from the meeting") that expands to its `[m:ss]`
  lines; a heard question is a capsule "What was just said (m:ss)"; the owner's questions are labelled **You**, answers
  **Persona** in `Theme.ai`, plain text in the room and Markdown in the follow-up; the end is a quiet "The meeting ended";
  the follow-up's seed is a collapsed capsule "The meeting's material", another context block one named "Context", its
  title heading what opens. A capsule is a button, so its words stay on one line: shorter at accessibility sizes ("N
  lines", "What was just said"), spoken whole. After the meeting, asks answered from the meeting's material stand where
  they were asked (by the hub's order of asks), before the meeting's end. A reply being written shows as a pending answer,
  a row of its own that alone redraws as the reply streams.
  The thread follows its end unless the reader scrolled up, then **Jump to Latest** shows with the count of unseen
  entries. Asks the room's thread doesn't hold (still asking, failed, answered from material) keep the existing
  `MeetingExchangeRow` look.
- **The After the Meeting divider**: a centered caption between two hairlines, the look of the chat's `CompactDivider`,
  between the room's thread and the follow-up's. With nothing before it (a recording that never had a room, a room that
  never came up and no asks) there is no divider: the thread is the follow-up alone, and until it starts one secondary
  line says what to ask ("Ask Persona about this meeting: what was decided, who does what, or a follow-up email."),
  shown once the hub has answered for the meeting, so it never flashes over a follow-up or room made elsewhere.
- **Hub transcription (phase 5)**: in a meeting the hub transcribes, the subtitle adds "Transcribed on the hub" (the
  mode is said on the page, as the start chose it). The Live box keeps its look and shows what the hub has while its
  audio socket is up and no problem other than captions missing stands: the hub's newest lines, then its caption being
  said now under them in the secondary color (read as "Being said: …"; no "behind" figure while a caption shows, folded
  to the caption alone while the keyboard is up). Otherwise it shows this iPhone's own lines with a notice under its
  title: one caption line, the iPhone mark (`Theme.Icon.phone`) in `Theme.caution` and why in words (waiting for Wi-Fi,
  the hub stopped transcribing this meeting, or it isn't transcribing right now), "Showing this iPhone's transcript". Not
  a `NoticeBanner`: it is about the box's lines, so it sits in the box rather than pushing the page down, and goes by
  itself when the hub is back. It never shows for what isn't a fallback: a pause, the end after Stop, or a reconnect of
  up to 5 seconds (a notice that flashes on every blip teaches people to ignore it). With Persona off, the phone's own
  lines and the hub's caption. With the Highlights card (phase 2b) on top, which shows no lines, the hub's lines are on
  the Full Transcript page, in the timeline's rows in place of the phone's (no gap marker under them: the caption says
  what is being said now), and its caption under the last row (never cut there); a fallback shows the phone's own lines
  there with the same notice above them, and the notice also sits under the card's title, as in the Live box, so the
  page says it without a tap. A highlight's time lights the hub's lines in its minute then. The Lock Screen's line is
  the hub's latest, or "On iPhone: …" with the iPhone mark once it fell back (docs/apple/live-activity.md "Recording").
  The recording's page says how it was transcribed in its header (`Theme.Icon.meetingTranscription`, a caption block:
  "Transcribed on the hub", "The hub is finishing the transcript…" with a small spinner until final, the stretches the
  hub has no audio for, the gap uploads), and its footer says the audio went through the hub, from what the phone knows
  until the hub's word arrives (never "the original audio isn't uploaded" while it may have been). Reason: the owner chose
  where the audio goes, so the page says it, and a fallback must never pass for the hub's text.
- **Talk about this meeting (phase 6)**: on the recording's page one row, in the section under the header beside
  Private Meeting, drawn as that row (a `Label` with the style's `Theme.Icon.conversation`, one line), pushing the after
  mode; it shows for any recording that ended (not rolling or all-day) with text, minutes or a conversation the hub holds
  (past retention not the text), on a hub with the meeting conversation. It is a row, not a button: the page's primary content stays the minutes, and
  the composer is the thread's. The minutes push opens the same page over Recordings, with Open to the recording's page,
  and the live sheet of a recording running then doesn't present itself over it.
- **Talk (phase 4)**: while a room meeting Persona can be asked in records (listening or taking part, not paused),
  **Talk** stands in send's place while the field is empty, as the chat's voice button does, so the row keeps one solid
  button (`MeetingTalkButton`, via `MeetingQuestionField`'s `instead`): a capsule in the composer's ink, `Theme.Icon.talk`
  and the word **Talk** on one line (only the icon at accessibility sizes, in the row under the field, its name the
  label). It takes a touch as every mic does (#9658, section 4.2 "Tap or hold a mic"): a tap turns it on and the next
  tap ends it and sends; pressed and held it is on until let go. The span opens as the finger lands either way, so the
  first words count; let go before `Theme.Hold.delay` it stays open as the page's talk window. On, it takes `Theme.ai` on
  `Theme.aiTint` and grows to `Theme.Recording.talkHeldScale` (no growth under Reduce Motion), and the caption under the
  field says "Listening to you… Let go when you're done." (held) or "…Tap Talk when you're done." (a window), announced
  for VoiceOver. The page's window never ends on a pause (Talk is in sight, and a pause to think is common in a meeting):
  it ends on the next tap, after 20 seconds without speech or at 60 seconds, and then the caption says why ("Talk turned
  off after 20 seconds without speech." / "…after a minute."); the Lock Screen's window keeps its 2 seconds after speech.
  A span asked without the call that held no words says Persona heard nothing; these words go after a few seconds. No
  slide-to-cancel: while Persona takes part the audio goes to the call as it is spoken, so a cancel could not take it
  back. VoiceOver's double tap opens the page's window, the next ends it; its hint says where the words go. Where a span goes to the voice service (taking
  part, the extension takes meeting calls) a caption line of its own under the caption shows the contract's privacy
  Wording (hub mode's in a meeting the hub transcribes, phase 5) until a span went while it showed ("Audio stays on this iPhone, except while Talk is on: what is said then
  goes to the voice service."), beside a word after a span rather than replaced by it, and the line under Record says it
  before the meeting starts. Until a span went under it, a span from anywhere (held, VoiceOver's, the Lock Screen's) is
  asked without the call unless that line had been on screen for a second as it opened; the Lock Screen has no room for
  the Wording, so its Talk waits until it was seen. Hidden with Just Me on (Talk is the room's) and after the meeting: a
  hold
  ends when Talk leaves the screen under the finger (Just Me, the page left, a word typed), and after 60 seconds; a
  window stays and ends by itself.
  While the meeting call's voice speaks, the page's **Stop Speaking** row shows and cuts it (`interrupt`), as it does for
  a spoken answer. Reason: Talk is the contract's way of addressing Persona (no wake phrase); a hold makes the
  span exactly what the owner said, and since #9658 a tap does too, closed by the owner's next tap rather than a pause
  (it was hold-only before: a tap did nothing, unlike every other mic); taking send's place keeps typing and talking one
  gesture apart without a second solid button. No haptic beyond the light tap as a span opens; nothing at its end, since the microphone is open.
- **Voice after the meeting (phase 6)**: once the follow-up exists, the composer has the chat's **Voice Chat** button
  (`ComposerVoiceButton`, the plain face) before the field, where the live page has [+]; at accessibility sizes it is in
  the row under the field. During the call the chat's `VoiceBar` takes the composer's place, and a call that ended by
  itself leaves the chat's `VoiceEndedStrip` over the composer, as in a chat (section 3.1). No voice before the follow-up
  exists, and none while something records.
- **Photos (phase 3)**: in a meeting started while the hub listed `meeting_photos`, [+] also has **Take Photo** (where
  the device has a camera) and **Choose Photos** (the system picker, images only, at most
  `Theme.Recording.photoPickLimit` at once), live only and never with Just Me on: a photo is the meeting's material,
  not a private question. Picked photos show in the thread at once as **Preparing…** rows, before Photos hands their
  bytes over; those it can't (still in iCloud) leave again and are said in one `Theme.danger` line under the field until
  the next pick (at accessibility sizes a danger `NoticeBanner` at the end of what scrolls instead, so the bar stays two
  rows). Each photo is a row of its own: a thumbnail (`Theme.Recording.photoThumbSide` square, scaled with the text,
  `Theme.Radius.control` corners on `Theme.Fill.well`, the photo icon until the picture is ready; a tap opens the
  viewer), then "Photo" and the meeting clock it was taken at, the caption or the first
  `Theme.Recording.photoTextLines` lines of the words read on it, and its state in words: **Preparing…** and
  **Uploading…** with a small spinner, **Used as material** with the success check in `Theme.positive` once the hub
  stored it, or **Couldn't upload** in `Theme.danger` with the reason. A failed photo has its buttons under it, plain
  themed ones: **Retry** only when sending it again can help (no answer, a busy or read-only hub, a full meeting once a
  photo is deleted), and **Delete Photo** always, so a refusal the hub would give again (a picture it can't take, a
  meeting whose transcript expired, a hub without photos, a device without access) never leaves a Retry that can't
  work. Under it the room's note shows as a **Persona** answer row; while the room has the photo and no note yet,
  "Persona is looking at the photo…" with a spinner, for at most `Theme.Recording.photoNoteWait` after it was added (a
  room that never writes one stops the spinner). After the meeting a photo the minutes cite says **Cited in the
  minutes** with the minutes icon. A photo whose room message the thread holds sits there; the others (not sent to the
  room, their message not among the room's pages loaded, still on their way) follow the room's rows, oldest first,
  those still on their way last; never hidden. A photo the room holds but the hub no longer lists (deleted, trimmed) is
  a quiet centered line, **Photo removed**, with no thumbnail. At accessibility sizes the thumbnail stands above the
  words, and the failed photo's buttons stack. VoiceOver reads the thumbnail, heading, words and state as one element
  (label "Photo, 12:34, its first lines"; value the state words, "Deleting…" while it is deleted, and Cited in the
  minutes), with **View Photo**, **Retry** and **Delete Photo** as its accessibility actions, so nothing needs the long
  press; the buttons and the note are elements of their own. Reason: the whiteboard is what the minutes and answers
  read, so the row says in words where it is (stored, read by Persona) instead of a badge, and the note shows what
  Persona took from it.
- **Deleting a photo**: the row's long-press menu has **Delete Photo** (destructive) for a stored or a failed photo, as
  does the failed one's button; then a confirmation dialog, **Delete This Photo?**, with **Delete Photo** and a message
  that says what deleting does: sent to the room, "Persona already saw it. It is told not to use it again, and it leaves
  the minutes."; stored but not sent, "It leaves the meeting's material and the minutes."; a failed one has no message,
  since it may have been stored with its answer lost: the delete reads the hub's list first and deletes it there when
  the list has it, else it only leaves this phone; with no list (offline) it stays and says why. While it is deleted the
  row is dimmed (`Theme.Recording.dimmed`) and its state reads **Deleting…** with a spinner, so the dimming never says
  it alone; a refusal shows its words under the photo in `Theme.danger` and is announced. Why a confirmation: it can't
  be undone, and it changes the minutes.
- **Photos and haptics**: adding a photo (camera or library) and deleting one play no haptic: the new row, its state
  words and the confirmation are the feedback, and the microphone is open (the Haptics line below keeps them to send,
  Pause and Bookmark).
- **A cited photo in the minutes** (the recording page's minutes: on iPhone the Summary's cards, `RecordingMinutesSummary`
  ("Minutes cards (#5026)" below), else `RecordingMinutesSections`; iPhone only, visionOS shows the item alone): an item that cites a photo (`"photo": 3`) shows that photo's thumbnail under its words, the
  same thumbnail as the thread's, a tap opens the viewer; **Photo removed** in caption secondary once the hub's list
  was read and no longer has it (a trimmed recording's deleted photo); the thumbnail's place while the list isn't read
  yet (offline), which is not a delete. Copy and Share end such an item with "(photo 3)", as the hub's Markdown does.
- **Photo viewer**: the thumbnail opens the photo full screen (`.fullScreenCover`) in the app's image viewer
  (`ImageViewer`), from this phone's copy or the hub's; a spinner with ✕ at the top left (#9653) until the bytes are here, and when
  neither has it an `EmptyStateView` ("Image didn't load") with **Retry**.
- **Just me**: a switch row in the bar (live only) with its caption ("Only you see it, in the main chat"). With it on the
  section header reads **Just Me** ("Only on this screen, never said aloud"), the field's placeholder "Ask privately…",
  and the thread shows only the owner's private questions and the main chat's answers to them, in the same You / Persona
  rows, with Sending…, a failure's words and **Retry** under a question. An answer is its turn's: the next user text in
  the main chat, the owner's or one the hub puts in (a task's notice, a scheduled run), ends it, and the main chat's live
  text shows only while the question's turn is the chat's newest, so nothing of another turn reaches a screen that may
  face the room. Its answers are never spoken, announced to VoiceOver, put in the Live Activity or pushed; a card the turn
  raises (one raised since its question, while its turn is the newest) is answered in the main chat, which the thread
  says in one line.
- **Page menu**: the **Persona** role picker, the **Private Meeting** switch (`PUT …/private`; a refusal shows its
  words, "Couldn't change Private Meeting. Try again." when the hub names no reason) and Meeting / Note
  ("One Record entry (#5243)" below). The mark shows as the hub has it
  once it said anything (absent: private); the start's choice stands in only until then. The start shows the same switch
  under the role, off by default, and no Who Attends. VoiceOver names the three menus apart: the page menu "Meeting
  options", [+] "Add to the conversation", the control row's "More".
- **On a hub without the room** (the #3746 start and live sheet): Who Attends never pre-selects the main chat. With
  nobody chosen the role shows Off; Listen or Take Part shows "Who Attends: Choose" with a caption under it in the same
  row ("Choose who attends. Until then Persona stays out of the meeting."), and the strip's Turn on Persona or role menu
  pushes the attendee picker first, turning on once someone is picked. The main chat stays an ordinary row of the picker.
- Haptics: only `tap`, on send, Pause and Bookmark, while the microphone is open (section 4.3); photos play none;
  new room answers are announced to VoiceOver as the live view's are.
- Identifiers for tests: `meeting-page-menu`, `meeting-role`, `meeting-private`, `meeting-page-subtitle`,
  `meeting-room-problem`, `meeting-live`, `meeting-transcript`, the highlights card `meeting-highlights` with each
  highlight `meeting-highlight` (its words and its time), its time `meeting-highlight-time` and its waiting line
  `meeting-highlights-waiting`, on the transcript a lit row `recording-timeline-sentence-lit` (selected while lit),
  `meeting-thread-header`,
  `meeting-thread-earlier`,
  one `meeting-thread-<kind>` per row kind (`lines`, `heard`, `question`, `answer` and the others),
  `meeting-just-me`, `meeting-just-me-retry`, `meeting-plus`, `meeting-heard`, `meeting-photo-camera`,
  `meeting-photo-library`, `meeting-photo-row` (the row), its photo element named by its state (`meeting-photo-preparing`,
  `meeting-photo-uploading`, `meeting-photo-material`, `meeting-photo-failed`, `meeting-photo-deleting`,
  `meeting-photo-unlisted`), `meeting-photo-retry`, `meeting-photo-remove`, `meeting-photo-note`,
  `meeting-photo-reading`, `meeting-photo-removed`, `meeting-photo-delete`, `meeting-photo-delete-confirm`,
  `meeting-photo-delete-problem`, `meeting-photo-viewer`, `meeting-photo-viewer-done`, `meeting-photo-problem`; the
  topics `meeting-topics` and each chip `meeting-topic-done|current|open`; in the minutes `minutes-photo` and
  `minutes-photo-removed`,
  `meeting-ask-field`, `meeting-ask-send`
  (the two shared with the live bar's typed question), `meeting-ask-caption`, `meeting-ask-notice`, `recording-more`,
  `meeting-page-stop-speaking`, `meeting-after-status`, `meeting-after-divider`, `meeting-followup-retry`,
  `meeting-followup-empty`, `meeting-voice` (and the chat's `voice-bar`, `voice-ended`); hub transcription's
  `meeting-live-hub-line`, `meeting-live-line` (the phone's own), `meeting-live-interim`, `meeting-transcription-fallback`,
  and on the transcript `recording-timeline-hub-line` and `meeting-transcript-interim`;
  at the start `meeting-private-start` and `meeting-transcription-start`; on the recording's page `recording-private`,
  `recording-talk` and `recording-transcription`; the sheet's
  shared ones (`recording-now-title`, `recording-live-timer`, `recording-live-hide`, `recording-pause`,
  `recording-bookmark`, `recording-stop`, `recording-wrapup-title`, `recording-wrapup-open`, `recording-wrapup-done`)
  keep their names. UI tests: `MeetingConversationUITests`, `MeetingLiveViewUITests` (the topics),
  `MeetingHighlightsUITests` (the highlights card in light, dark and the largest text size, a time opening the
  transcript lit, the keyboard, the waiting line before the first highlights and with the keyboard, a hub without the
  flag, a meeting started without it, a room that is unavailable, Persona turned off before any; no transcript on the
  card),
  `TalkAboutMeetingUITests`, `MeetingHubTranscriptionUITests` (hub transcription, in the Live box and with the
  Highlights card).
- Platforms: the web shows the same thread on the recording detail (**Conversation**, with **Talk about this meeting**
  from phase 6); Android has no meeting page yet.

## One Record entry (#5243)

Every place that starts a recording on the iPhone and the watch says **Record** and does one thing: it records a
meeting (minutes, and Persona can attend with the hub's default role), named after the calendar event on now when there
is one, or opens the recording running. That is the Recordings page's button, the main conversation's + list, the home
screen's menu, the Control and Siri / Shortcuts, and on the watch its recording page's button and its complication.
Reason: two large buttons, **Meeting Recording** and **Start Recording**, looked alike and owners couldn't tell which
to press; impromptu talks are meetings too (principles 1 and 4).

- **Meetings, not Recordings, on the iPhone** (#8731, D5): the place is **Meetings** (会议 / 会議, the meeting icon) and
  every start says **Start Meeting** (开始会议 / 会議を始める): the page's button (**Start Note** once Note is picked), the +
  list, the home screen's menu, the Control and Siri / Shortcuts, whose Record phrases keep working beside the new ones.
  While something records, the drawer's row and the + list say **In a Meeting… Open** for a meeting and **Recording…
  Open** for a note (`MeetingWords`). An unnamed note is **Note · date**. The watch keeps Record for now. Reason: a page of
  meetings named Recordings read as a tape recorder with settings, not a meeting (the owner, #8731).

- **The drawer's row is a place, not a start** (#5517): **Recordings** (录音 / 録音), first among the drawer's places,
  opens the Recordings page, as each row under it opens its page; Record there starts. While a recording runs the row
  reads **Recording… Open** and opens it. Reason: the row said Record and started at a tap, while in zh-Hans and ja it
  reads as the page's name, so owners tapping it to see their recordings got one running (principle 4: a drawer row
  goes somewhere; a verb belongs on the control that acts).

- **The Recordings page**: Record is the one prominent button (`.themedButton(prominent: true)`, one line, the icon going
  at accessibility sizes), with the calendar line under it. The options follow as one row each: **Type**, a menu
  `Picker` (Meeting / Note, Meeting for every start, never remembered), then for a meeting Persona, Private Meeting or Who
  Attends, **Transcription** (On This iPhone / On the Hub, a menu row with `Theme.Icon.meetingTranscription`, the hub's
  default shown until picked, for that start only; only on a hub with hub transcription that can transcribe now, #3862
  phase 5, so it never offers what can't be had), then language and microphone. The line under the options says what
  the start sends: hub mode's privacy Wording (the extension named when the owner chose another) once On the Hub is
  picked or is the default. Note hides the meeting's options and the calendar line and shows one caption
  ("No minutes, and Persona stays out."); the same Record button then records a note (progressive disclosure, principle 2).
  Note is never a second button (section 3.7).
- **The Meetings home** (#8731 v2): today's calendar meetings first (**Upcoming Meetings**, one row each: time in
  the meeting's red while it is on, name, people and place, and a **Start** that is prominent only for the one on now),
  then the **quick start**: the name as a text field (its prompt the calendar's or "Meeting · now"), **Meeting | Note**
  as a segmented control, who is in it (you and **Invite Persona**; Persona invited shows its face and "Answers on
  screen", its menu removes it), and one line for who transcribes (this iPhone, or "Cloud · service"; the warning tint
  when the hub's default can't run). The rest of the start's options are the page's ⋯ (private, language, microphone).
  **Start Meeting** stays the page's one prominent button. Reason: the owner asked for a meeting app, not a settings
  form ("design it as a meeting app", principles 1 and 2).
- **One home for meetings** (#8731): the page lists only what the knowledge base doesn't have whole yet (**Still on This
  iPhone**, each with its state word), then one row, **All Meetings**, into Knowledge's Meetings list. That list is the
  one place for every meeting and note, the imported ones and Persona's together, newest first (Persona's never fold
  under Not Yet Organized: one just ended has no summary yet); only notes and all-day
  recordings carry a pill (the exceptions), and Knowledge has no Recordings chip. A recording this phone has opens its own
  page from there and deletes with a swipe, asked first. Reason: the same list sat in two places (the page and
  Knowledge's last chip, start controls included), and after the rename two Meetings lists would split meetings by how
  they arrived (principles 2 and 4).
- **One row for one meeting** (#8872): a meeting both imported from a connected app and recorded in Persona is the
  recording's row (its minutes, transcript and map are Persona's), once the hub has matched them (after the minutes
  step; until then both rows show). Its avatar carries the app it came from in the corner, drawn as the Inbox marks a
  connector's item (the app's own mark on white, a gray link without one: `KBImportedBadge`, the web's `RowTile`, Android's
  `KnowledgeImportedAvatar`), and VoiceOver / TalkBack reads the notes' name as the row's value. The recording's page, on
  Knowledge's page and on the meeting's own
  after the meeting, has one row more under the facts: **Lark Meeting Notes** (another app's, **Original Meeting Notes**),
  which opens them in this app (`KBImportedNotesButton`, `kb-imported-notes`). Two recordings of one meeting both stay,
  the imported row goes; a recording naming a meeting the list doesn't have stays as it is (principle 4: one meeting,
  one row).
- **Hand to Agent after a meeting** (#8852): the meeting's page after it (the iPhone's **⋯**, the web's button beside
  Rename, Android's **Hand to Agent** toolbar action) offers **Hand to Agent…** only once the meeting is over and has
  something to hand, and only to someone with an Agent that can take it (principle 2: someone who only uses the assistant
  never sees it). The sheet is the to-do's Hand to Agent on the iPhone and the Inbox's Hand to AI on the web and Android
  (`HandOffTargets` / `handOffTargets`: the same Agents, the same rows): the Agent (the latest picked), an optional line,
  **Include the transcript** off by default, and one line on what the Agent gets. The latest hand-off is one row at the
  top of the page (`MeetingHandoffRow`, `RecordingHandoffRow`): where it went with **Open**, or why not with **Retry**,
  which opens the sheet again on that Agent (principles 3 and 7: the try shows at once, and a failure can be tried again).
  On Android the sheet's visibility and Retry context live in the page's ViewModel across Activity recreation, the
  background row stays out of the sheet's accessibility tree, and a failure reason wraps in full at large text sizes.
  No toast besides: the row says it, and on the iPhone a toast covered the meeting's title.
- **While it records**: the live sheet's page menu ends with the same choice, Meeting and Note with their marks (a menu
  section, the current one checked), so it can change after the start (principle 3): the meeting page's menu, and in the live view a page menu beside **Hide** (the ellipsis, named
  "Recording options" for VoiceOver, apart from the Persona strip's "Persona" and the bar's "More"). Turning a meeting into
  a note turns Persona off, and the sheet shows the live view; turning it back makes it a meeting with Persona's part as a
  start would get it, and a room meeting's page again.
- **Settings › Meetings** (#3862 phase 5) adds, from a hub with hub transcription, a **Transcription** section: the
  default as an inline picker (as Persona in Meetings is), what it sends in the footer; **Speech Service**, a menu row,
  only when an extension beside the built-in one can transcribe; **Glossary**, a row with its count that pushes a list
  page (a field with **Add** on its row, the newest term first, swipe to delete, a refused term said in `Theme.danger`
  under the field; one change at a time: while one saves, Add, Return and swipe do nothing and the field keeps its text,
  which empties only once the hub kept the term). **Hub Transcription on Wi-Fi Only**, a switch in a section of its own (it is this iPhone's, not the
  hub's), with what it does in the footer. Identifiers: `settings-meeting-transcription` (the picker; its options go by
  their words), `settings-meeting-transcription-service`, `settings-meeting-glossary`, `settings-meeting-wifi-only`,
  `settings-meeting-transcription-not-ready`, `glossary-field`, `glossary-add`, `glossary-term`, `glossary-problem`.
- Identifiers for tests: `recording-meeting-start` (Record), `recording-kind` and `recording-kind-detail` at the start,
  `recording-page-menu` and `recording-kind-now` while recording; `sidebar-meeting-recording` for the drawer's Recordings row. UI
  tests: `RecordEntryUITests`.
- **The watch** (#5493) has the same one entry, sized for a wrist. Its home list's row is a place, **Recordings**
  (`WatchMeetingEntries`), opening the recording page; "Recording… Open" while a recording runs. On that page
  **Record** is the one prominent button (`.borderedProminent`, one line), always a meeting. **Type: Meeting** under it
  is a text button (`.borderless`, the secondary font) with the kind's mark, as the iPhone's Type row: Record's
  qualifier, never a second start button, with no capsule of its own, so Record stays the one large button at the top.
  A tap switches it to **Type: Note**, which shows "No minutes, and Persona stays out." under it, for that one start;
  never remembered. Why a tap and not a picker: two values, one tap each way, and a watch picker pushes a page or fills
  the screen with a wheel. The **Record** complication stays a one-tap start of a meeting, as the Control on the
  iPhone. While all-day recording runs, Mark Meeting stands in for every start (one microphone). Identifiers:
  `watch-meeting-recording` (the row), `watch-recording-start` (Record), `watch-recording-kind` and
  `watch-recording-kind-detail`. The watch is dark only (section 6.1), so its screenshots have no light variant.

## Checking typed credentials by themselves (#4881)

Where a form exists to connect to something with an address or key the user types (the web provider wizard's preset
key step and its custom gateway's Connect step), the check runs without a button: once what it needs is valid and has
stopped changing for 0.7 s, it runs once for that input. An address being typed counts only once focus moves from its
field to something else on the page (not when the window loses focus), the step is left, or a paste fills all of it,
so a key never goes to a half-typed host; until then the form says leaving the field starts the check. It never runs
on opening something already saved, only after the fields the form names as its connection change (the provider
wizard: address, key, sign-in method, request headers); fields that play no part in the connection (a name) never block or repeat it.
The optional request-header editor stays visible on the custom gateway's Connect step before the check, in add and edit,
with configured names but never saved values and with the selected runtime's support or limitation beside it. Reserved or
unsupported headers stop the check instead of letting it imply a launch would send them. A header-only edit invalidates
old results and starts the same 0.7 s check. Each part runs whatever an earlier part said, since one part failing does not prove another will (a key may chat
without listing models); a part that is running or passed is not sent again. A line says it will check by itself, then
shows progress with a spinner, then the result; a failure keeps the server's words and offers to run that part again
(Retry on the Connect step). Manual buttons, where the form has them, and the note that a test may cost a little stay;
pressing one counts as the check. Once the check found what the next step needs, that step's button is the one primary
action (section 3.7). Reason: the user's next question after pasting a key is whether it works, and the preset path
already answered it this way; two paths in one dialog should not differ. This reuses the existing Button, Spinner and
status-line styles and adds no setting.

## Provider display names (#5029)

Wherever the web names a model provider or launcher (New Agent's choices and presets, the model settings, the
profile's Agent presets, the work-session and to-do launcher menus, task and schedule cards, a computer's launcher
list, toasts and dialog titles), it leads with the provider's display name in the element's normal text style and shows
the startup ID as secondary text: small monospace in `--ink-3` where a row has room, the `title` tooltip on chips and
buttons, "Title (id)" in a native `<option>`. A launcher with no display name (an rc alias, plain `claude`, a provider
from an older hub) shows its ID in monospace as before. One module decides it, `web/src/lib/provider-names.ts`; a page
that holds only an ID looks the title up there, and a row that shows the ID beside the name draws both with
`ProviderName` (`web/src/ui/ProviderName.tsx`). The provider wizard asks for the display name first (any language, at
most 64 characters; left empty, the hub takes the address's host) and keeps the startup ID under More options, where
the hub makes one up when it is left empty and its format is checked only once something is typed. A saved provider can
be renamed; its ID can't change, and the edit page says why (launches, Agent presets and the model settings refer to
it). Reason: the ID is an engineering detail that a person should not have to invent or read (principle 2), and one
rule for every place keeps a provider recognizable wherever it is picked (principle 4). This reuses the wizard's
fields and More options disclosure and the existing row and chip styles; it adds no setting, style or motion. The chat
page's composer chip and model panel adopt the same module in their own change; iOS and Android have not caught up yet.

## New Agent's model choice (#5104)

New Agent picks its model with the chat's model pill and unified model panel ("Model pill and unified model panel" in
3.8), at the user's request (2026-11-12) to match Cindy's new task: the user picks a model and the launcher and its
engine follow; the engine's mark stays on the pill and on every row. Who it serves: people starting coding Agents (the
Agents side). The pill sits at the right of the first message box's bottom row; the Preset, Launcher, Model and Thinking
effort fields are gone. What the panel gained for it, each part optional and absent from the chat's panel:

- Follow row (`follow`): on top of the list in no group, a 1 px `--line` rule under it; a two-line row (its label, then
  what the default runs in 12 px `ink-2`, after a 11 px `warning` icon in `--orange` when that can't start here), with
  the selected fill and a 14 px check while followed. Keys and the hover glide as on a model row.
- Presets (`presets`, `onPreset`): a group after Favorites, "Presets"; one row per preset named by the preset, its
  description as the second line, the launcher's tile and the `[engine] effort` setup on the right; no star, no Customize,
  never lit (a copy of a setup, like a favorite).
- `state.starting`: no Recommended group, and the row lit shows the form's setup rather than a saved customization.
- Engine pills in the flyout (`engines`): when the model runs with more than one engine on this computer, Cindy's engine
  pills replace the static engine block: 26 px tall, no border, a 12 px `AgentMark` and the engine's name at 12 px, the
  row's own on `--hover-2` with a check; another is a quiet pill that picks that launcher's row of the model, at 50 % with
  its reason when that launcher can't start here.
- "How to fix" (`onHelp`, a provider's `help`): after a launcher's reason under its group's name, 11 px medium
  `--accent-ink`.
- Effort with no level set (`EffortSlider` `unsetLabel`): no fill, the thumb a hollow ring (`--surface` with a
  `--line-strong` border, no glow) at the start, the bubble and accessible value "Model default"; a click or key picks a
  level.
- Lines under the first message box: one 11.5 px line each, a 12 px icon first: a pick dropped on another computer
  (`info`, `ink-3`), the computer's launchers that couldn't be read (`warning`, `--orange`, the reason in `ink-3`, Retry
  in `--accent-ink`; #10952), a list that couldn't be read (`warning`, `--orange`), what runs this time can't start here
  (`warning`, `--orange`, the "Used this time" pill, How to fix), the preset picked (`layers`, `ink-3`; `warning` in
  `--orange` when something of it can't be applied).
- Launchers that couldn't be read (#10952): the pill leaves its spinner for its failed state (the red `warning` and
  "Model loading failed"), and a click on it reads them again, as Retry on the line under the box does; Start still runs
  the computer's default.

Reason: one place to choose a model for chats and new Agents (principle 4), the engine visible without a field of its
own (principle 2), the default as one row that can always be gone back to (principles 1 and 3). The deviations from
Cindy are listed in `docs/web/web.md` "New Agent's model choice (#5104)"; iOS and Android follow in #5117.

## Permission modes a provider lists (#8261)

Who it serves: people starting and steering coding Agents (the Agents side) on providers other than Claude Code, such as
Paseo's Grok, OpenCode or Codex. Claude Code's five modes mean nothing there: the bridge refuses Plan, drops the rest,
and the provider's own settings decide what it asks. One rule on every client (web, Android, iOS), read from the wire
rather than from provider names:

- **New Agent**, for a launcher that lists `permission_modes` (`docs/protocol/protocol.md`, `launchers.list`): the
  Permission mode field offers **Launcher default** (the launcher's own approval, sent as no mode) and **Allow all**
  only where it lists both, in each client's existing words for its follow-the-launcher choice and for bypassPermissions
  (the web: Launcher default, Full access; Android: Follow launcher, Allow everything; iOS: Use Launcher's, Allow
  Everything); a launcher with its own approval alone shows no field. The pick is made on the sheet and never
  remembered, so a mode left from a Claude Code start never reaches such a start. The effort offers only the levels its
  model lists, none while that is not known. A Claude Code launcher keeps its five modes and remembered choice.
- **A running chat**, for a session that lists `capabilities.permission_modes` with two: the client's existing mode
  control (the web's composer permission picker, Android's and iOS's mode menu) with those two rows alone. "Launcher
  default" carries a neutral mark (the plain shield, never the raised hand of "asks first"), since the provider's own
  settings may approve everything; no mode reported is Launcher default. Going into Allow all asks first, as it does for
  Claude Code; leaving it does not, since it can be turned on again. No Plan mode row.
- **Session info** leaves out counts nobody gives (`usage_unknown`) rather than showing zeros.

Reason: the same start and the same running chat behave the same on every client and entry point (principle 4), the
default is the launcher's own and nothing is sent unless the user picks it (principle 1), and Allow all stays a
confirmed, reversible choice (principle 3). It reuses each client's mode field, picker, menu rows, warning text and
confirmation; it adds no component, token or motion. `capabilities.modes` stays false on these rows, so clients
older than the list show no Claude Code chips there.

## Minutes cards (#5026)

A recording's page on iPhone, once its minutes are ready (`docs/apple/recording.md`, "Minutes"): a segmented
**Summary | Transcript** switch right under the page's header, Summary by default, the choice kept while the page is open
(no setting). Before the minutes are ready the page has no switch and is as it was. Reason: a meeting's page is opened for
what was decided, and the transcript stays one tap away (principle 2) instead of being scrolled through line by line.

- **Summary** (`RecordingMinutesSummary`, `apple/Persona/Recording/RecordingMinutesSummary.swift`): cards that are
  sections of the page's inset grouped List, one row each with `Theme.Space.l` insets, so they have the page's other
  sections' background and corners in every style. Not `.surface` cards: a surface's shadow drawn in a List row is cut
  off square by the row's edges. First a header card (the minutes' title
  when it differs from the recording's, in `Theme.Font.heading`; when · where and the attendees as secondary lines with
  `ThemedIcon`), then one card per group with something in it: Key Points (`Theme.Icon.keyPoint`), Decisions
  (`Theme.Icon.decision`), Open Questions (`Theme.Icon.openQuestion`), Action Items (`Theme.Icon.todo`). A card's header
  is its icon in the accent, its title in `Theme.Font.heading` and its count as a neutral `Chip`, read as one heading. Its
  items follow with a hairline between them: the words in `Theme.Font.body`, for an action "who · by" in
  `Theme.Font.caption`, secondary.
- **A cited photo** (#3862 phase 3): an item that cites a meeting photo shows it under its words as in the minutes'
  sections ("A cited photo in the minutes" above): the same `MinutesPhoto` thumbnail, a tap opens the viewer, **Photo
  removed** once the hub's list no longer has it. Copy and Share keep "(photo 3)".
- **To-dos and Regenerate** (#4062): each item of the Action Items card has **Add to To-Dos** (a `.themedButton`, one
  line) or the positive **Added** chip under its words; the card ends on **Add All** while more than one is left. The
  header card carries what is happening to the minutes (`RecordingMinutesUpdates`): "Updating the minutes…" with a
  spinner, a failed rewrite in `Theme.danger`, an expired transcript, and **Regenerate** as a `.themedButton` (in a card
  every control is its own button, never a row that takes the tap).
- **An item's time** is the compact accent `Chip` (`RecordingFormat.clock`) inside a 44 pt hit area: a tap switches to
  Transcript, scrolls to that line and lights it (the page's existing jump), read as "Show at m:ss in the transcript".
- **An item's disclosure**, the chevron beside the time (`Theme.Icon.expand` / `collapse`, 44 pt), opens the transcript
  lines around its time in place, on an inset well (`Theme.Fill.well`, `Theme.Radius.control`) as the meeting thread's
  opened lines: the line `RecordingMinutes.sentenceIndex` picks in the primary color and the stronger weight, with
  `Theme.Recording.contextBefore` (1) line before it and `Theme.Recording.contextAfter` (2) after it in the secondary color.
  They fade in on `Theme.Motion.reveal` and out on `Theme.Motion.conceal`, at once with Reduce Motion. VoiceOver hears
  "Transcript around m:ss" with Expanded / Collapsed.
- An item's time and disclosure are a card's controls and stop growing at xxxLarge (6.2); its words and its opened lines
  are reading content and grow to AX5. Up to xxxLarge the time and the disclosure sit beside the words. At the
  accessibility reading sizes they go under the words, which then keep the card's whole width, and an opened line's
  time goes above it: `MinutesItemRow` and `MinutesContext` decide this from the size the words read at
  (`\.readingTypeSize`), not the interface's capped size. The summary card's items go by width instead
  ("By the width that is there" below).
- **Transcript** is the transcript as it was; the minutes' sections are not repeated above it. The minutes' **Copy** and
  **Share** sit under the switch on the page's canvas, the same in both views; at the accessibility sizes they stack, so
  neither is cut to "…".
- What the Summary shows (which cards, their items, an item's lines, the default view) is `RecordingMinutesCards`
  (`apple/Shared/Recording/RecordingMinutes.swift`), worked out when the minutes or the transcript change, never in `body`.
- Identifiers: `recording-view-switch` (the switch; its segments `recording-view-summary` / `recording-view-transcript`
  where the system carries them, else by position), `minutes-card-header`, `minutes-card-points`,
  `minutes-card-decisions`, `minutes-card-open`, `minutes-card-actions`, `minutes-time`, `minutes-item-context` (the
  disclosure), `minutes-item-lines` (the opened lines); `minutes-copy`, `minutes-share`, `minutes-when`, `minutes-photo`,
  `minutes-photo-removed`, `minutes-add`, `minutes-added`, `minutes-add-all`, `minutes-updating`, `minutes-regenerate`,
  `recording-sentence` and `recording-sentence-lit` keep their names. UI tests: `MinutesCardsUITests` (light and dark,
  the default and the largest text size, a cited photo) and `MeetingRecordingUITests` (to-dos, Regenerate, the date
  style).
- Platforms: visionOS keeps the minutes' sections above the transcript; the web shows minutes that carry an overview as
  the summary card below and has no Summary | Transcript switch; Android has no Summary yet.

### Web: the summary card (#6563)

Who it serves: everyday people reading a meeting's minutes in the browser. Minutes that carry an `overview` read as one
summary card (contract: `docs/assistant/meeting-minutes.md` "Summary card (#6563)"); the others look exactly as before,
pixel for pixel. Reason: where the meeting landed and each workstream's state are what a reader looks for first, and
the flat Key points / Decisions lists made them read every line (principle 2).

- **Where.** `web/src/lib/minutes-overview.ts` reads the overview on its own (`minutesOverview`, the contract's
  "Clients" cleaning) and finds the bold places (`boldParts`); `RecordingMinutes.tsx` chooses the layout once, when the
  minutes change (a bullet left after cleaning), never while drawing; `web/src/views/knowledge/MinutesOverview.tsx` draws it.
- **Card.** Still one `Card` in the recording page's column. Its first child holds the headline (or the minutes' title)
  as the h3 in `text-card-title`, when · where · attendees on one line in `text-label` `--ink-2` (the dots are read out
  too, so a screen reader keeps the facts apart), the partial note,
  then **Core conclusions** on `bg-ai-tint` (`rounded-card`, the `light` icon in `--ai`, an h4; these are the AI's own
  words), then the groups. The topic area is that one child, so no divider runs between topic cards. Action items (the
  page's `ActionItems`, Add to to-dos / Added / Add all unchanged) and Open questions (`PointList`) follow as h4
  sections, then the AI note (`sparkles` in `--ai`, `text-label` `--ink-3`). Key points and Decisions are not shown again.
- **Groups.** A titled group is a `section` labelled by its h4 (`text-label` semibold `--ink-2`, a dot in its tint),
  its topics h5. The single untitled group has no heading: a plain grid, each topic an h4. Each topic is an `article`
  labelled by its title.
- **Topic card.** `rounded-card` on `--topic-N-tint`, N by the group's position (the first group the first tint, four in
  turn); a tile of `bg-surface` with the topic's glyph in `--topic-N`, its title in `text-meta` semibold. A bullet is a
  dot in `--ink-3`, or for a decision the `flag` in `--ink-2` and the word **Decision** where a point shows its speaker
  (`text-label` `--ink-3`), so nothing rests on the glyph or the color. That label goes to the next line whole when it
  does not fit after the words, and only a name longer than the line wraps (an `inline-block` as wide as the line at
  most). The words are `text-meta` `--ink` with the
  `strong` places `font-semibold`; no text is read as Markdown. The time or the cited photo is the page's own
  (`TranscriptTime`, `PhotoCite`): beside the words while they keep 12em, else under them on the right; disabled when
  there is no transcript line (the trim), a kept photo still opens under Photos. After 4 bullets the card shows
  **Show N more** / **Show less** (`Button` quiet, small, `aria-expanded`), which open the rest in place. The button
  controls the topic's list (`controls`, aria-controls) and its name names the topic, its visible words first ("Show 2
  more in Export", `label`), so several topics' buttons differ. A topic's fold is kept by the minutes' `version` and its
  title: a confirmed to-do keeps it, rewritten minutes start folded.
- **Grid.** `minutes-topic-grid`: wrapped rows of as many cards of `--minutes-card-min` as fit, never more than three
  (a card is at least a third of the row), each card growing to fill its row and the cards of a row sharing its height.
  Two in the 720 px column, at the Large text size and at 200% browser zoom too; one on a phone. A card alone in its
  row (the third of three, a group's only one) takes the whole row, where a grid's `auto-fit` left it half a row beside
  an empty cell. The reading order runs row by row.
- **Text size.** Every size is a named one (`text-card-title`, `text-meta`, `text-label`); the card's own marks, tiles
  and icons are sized in `em` and `--minutes-card-min` follows `--text-scale`, so they grow with the page's text.
- **Tokens** in `web/src/views/knowledge/minutes-overview.css`, which loads with the Knowledge page: as Tailwind
  utilities in `web/src/styles.css` they added 325 B to every language's first screen (`FIRST_SCREEN_BUDGET`) for a page
  most visits never open, so the card's tokens and its few own rules (the tints by `data-minutes-tint`, the grid, the em
  sizes) live there, as `web/src/ui/share-tokens.css` does for sharing; everything else is the usual utilities. Checked in
  `web/src/ui/theme-contrast.test.ts` (`--ink`, `--ink-2`, `--ink-3` and `--accent-ink` at 4.5:1 on every tint, each hue
  at 3:1 on its tile, `--ai` and the words at 4.5:1 on `--ai-tint`; the times for the page's own accent, every built-in
  theme color, and a sweep of persona and extension accents through `accentTokens` and `meetContrast`):

  | Token | Light | Dark | Use |
  | --- | --- | --- | --- |
  | `--topic-1` … `--topic-4` | `--hue-blue`, `--hue-teal`, `--hue-pink`, `--hue-gray` | same | a topic's glyph and its group's dot; never a status hue or `--ai` |
  | `--topic-1-tint` … `--topic-4-tint` | `#ebf5ff` `#ecf6f7` `#fcf0f5` `#f5f5f5` (the hue at 8% over `--surface`) | `#26303d` `#263337` `#372d34` `#2f3034` (10%) | the topic cards (`.minutes-topic` under `data-minutes-tint="N"`) |
  | `--minutes-card-min` | `calc(15.5rem * var(--text-scale, 1))` | same | a topic card's least width |
  | `--minutes-card-gap` | `0.75rem` | same | the grid's gap |
  | `--minutes-ai-accent-ink` | not set (`--accent-ink`) | `color-mix(in srgb, var(--accent-ink) 85%, var(--ink))` | a time in the core conclusions |

  On `--ai-tint` dark `--ink-3` is under 4.5:1, so inside the conclusions it takes `--ink-2`
  (`.dark [data-minutes-conclusions]`), as the dark user bubble does. The dark AI tint is also lighter than every page
  background a theme color's `--accent-ink` is made readable on (`meetContrast`), so every built-in theme color's times
  fell to about 4.3:1 there: inside the conclusions a time takes `--minutes-ai-accent-ink`, its `--accent-ink` 15% of the
  way to `--ink` (4.6:1 at worst over the sweep; the page's own blue barely changes). The topic tints need nothing:
  light they are lighter, dark darker, than the backgrounds `--accent-ink` is checked on.
- **Topic glyphs** (`TOPIC_GLYPH`; any other value is `general`): progress `trendUp`, schedule `calendar`, release
  `send`, performance `gauge`, metrics `chart`, tooling `tool`, quality `shield`, design `layers`, people `users`,
  customer `person`, budget `money`, risk `shieldAlert`, general `list`.
- **Copy** sends the hub's `markdown`, as it always did.
- Checks: `web/src/lib/minutes-overview.test.ts` (the cleaning, `boldParts` and the shared
  `spec/fixtures/minutes-overview/cases.json`), `RecordingMinutes.test.tsx` (order, heading levels, fallback, the fold,
  to-dos, Copy, the trim), `RecordingPhotos.test.tsx` (a cited photo after the trim) and `web/e2e/recording-minutes.mjs`
  (en, zh-Hans and ja at 1440 and 390, light and dark; the Large text size and a 1440 window at 200% zoom; a card
  alone in its row and a long speaker name, with every topic card and line checked for running past its box; and the
  legacy baseline).

### The summary card (#6563)

Minutes that carry an overview (`docs/assistant/meeting-minutes.md`, "Summary card (#6563)") show it in the Summary
instead of the four cards; minutes without one, or whose overview has no bullet left after the contract's cleaning, keep
the four cards pixel for pixel. Which layout is settled in `RecordingMinutesCards.summary` (`Summary.overview`), never
while drawing. Who it serves: everyone who opens a meeting to see where it landed (principle 2): the headline and the
conclusions first, the details by topic, the transcript one tap away.

- **Order** (`RecordingMinutesOverview`, `apple/Persona/Recording/RecordingMinutesOverview.swift`): the summary card, the
  topic cards under their group headings, Action Items, Open Questions, then the AI note. Points and decisions get no
  cards of their own. Every card is a section of the page's List as the four cards are (`minutesCard()`), never a
  `.surface`. One column on every iPhone; iPad and landscape grids come later.
- **Summary card**: the headline in `Theme.Font.title` (a VoiceOver heading; the minutes' title when there is none and
  it differs from the recording's), when · where and the attendees only where the page's header
  (`RecordingMeetingFacts`) does not say them (`RecordingMinutesCards.Shown`; the line's icon `Theme.Icon.calendarEvent`
  with a time, `Theme.Icon.place` for the place alone, as the header's), then the **Core Conclusions** box: an inset
  well on `Theme.aiTint` (`Theme.Radius.control`, `Theme.Space.m` padding), headed by `Theme.Icon.conclusions`
  (lightbulb) in `Theme.ai` and its title in `Theme.Font.secondaryStrong`, primary (the AI color as words is under 4.5:1
  on the tint), read as a heading; each conclusion its words with the bold places, its photo and its time as a compact
  `Chip` with primary words on `Theme.aiTint` (`MinutesConclusions.chip`: the AI color as words falls under 4.5:1 on the
  well, the accent's chip under 3:1), with no disclosure. Updating, a failed rewrite, an expired transcript and
  **Regenerate** follow, as in the four cards' header.
- **Group heading**: the section header of a group's first topic card, a dot (`Theme.Icon.dot`, `Theme.Font.heading`,
  small) in the group's tint and its name in `Theme.Font.heading`, primary, as written (no text case); it wraps. None
  for the one untitled group.
- **Topic card**: its icon (`RecordingMinutesCards.symbol`, a `Theme.Icon.topic…` per `icon`, `topicGeneral` for any
  other value) in the group's tint on a soft tile of it (`Theme.softTint`, `Theme.Size.wellS`, `Theme.Radius.control`)
  and its title in `Theme.Font.heading`, the heading VoiceOver reads (the tile is hidden from it), side by side; at the
  accessibility sizes the tile goes above the title, which then starts at the card's edge
  (`MinutesTopicCard.tileAbove`). Then its bullets as the four cards' items (`MinutesItemRow`: the words in
  `Theme.Font.body` with the `strong` places in bold through `RecordingMinutes.bolded`, never Markdown; the photo; the
  time chip; the disclosure and its lines), each after its mark in a column of `Theme.Size.markM`: a point's dot (`•`,
  `Theme.Font.bodyStrong`) or a decision's `Theme.Icon.decided` (`Theme.Font.body`, small) in the group's tint, both
  hidden from VoiceOver. A point's speaker or the word **Decision** (`Theme.Font.captionStrong`) goes under the words,
  so nothing rides on the glyph or the color. The first `Theme.Recording.topicBulletsShown` (4) show; **Show N More** /
  **Show Less** (`Theme.Font.captionStrong`, the accent, one line, a 44 pt height) opens the rest in place on
  `Theme.Motion.reveal` / `conceal` (at once with Reduce Motion); VoiceOver hears "Show N more in <topic>".
- **Tints**: `Theme.Hue.minutesGroups` (blue, teal, pink, slate), by the group's place (`Theme.Recording.topicTints`),
  never chosen by the model, never a state's color (red, orange, green, yellow) or the AI's. `StyleSpecTests` holds
  every tint at 3:1 or better on every style's card and on its own tile, and on the conclusions box the lightbulb at
  3:1 and every word at 4.5:1, a conclusion's time on its chip included, in light and dark.
- **Action Items** keep Add to To-Dos, Added and Add All. An added action whose to-do the app holds (`TodoStore`) starts
  with that to-do's done circle (`MinutesTodoCircle`: `Theme.Icon.unchecked` / `successFilled` in `Theme.Font.heading`,
  `Theme.positive` when done, a 44 pt column, hit at least 44 pt tall through `tapTargetOutset` without a taller row),
  which shows and ticks it as the To-dos list does (5.6): the list's `TodoTriageLogic.checksAsOpen` with its triage, so
  a failed start still waiting on the owner reads open and a tick closes it. VoiceOver hears a switch named by its
  action ("Done: <action>"). A suggestion never shows a box; the column is there once an action is added (empty until
  the app reads its to-do), decided from the prepared items, and only the circle reads the store (P5).
- **AI note**: the last section's footer, `Theme.Icon.ai` in `Theme.ai` and "AI-generated from the recording. Check
  anything important." in `Theme.Font.caption`, secondary: the same words as the hub's Markdown. Only in this layout.
- **Copy and Share** send the hub's `markdown` in this layout (`RecordingMinutes.shareText`); the four cards keep the
  phone's own text.
- **By the width that is there** (`MinutesItemLayout`, 1.2): an item's time and disclosure sit beside its words' first
  line while the words keep `Theme.Recording.itemWordsColumn` (7) times their line's height of width; else the words go
  across the item and who said it (or Decision), the time and the disclosure share the line under them, as the
  contract's sketch draws a bullet. The words are reading content and grow to AX5 while the interface stops at
  xxxLarge (#6813), so their room decides, with no accessibility-size branch. Conclusions, bullets, and the Action Items'
  and Open Questions' items in this layout all follow it; the four cards' items keep theirs. The headline and the
  titles are interface text; group headings wrap. VoiceOver headings, in order: the headline, Core Conclusions, each
  group, each topic, Action Items, Open Questions.
- Identifiers: `minutes-card-summary`, `minutes-headline`, `minutes-conclusions`, `minutes-conclusion`,
  `minutes-group-<g>`, `minutes-topic-<g>-<t>`, `minutes-point`, `minutes-decision`, `minutes-topic-more`,
  `minutes-topic-less`, `minutes-todo-open`, `minutes-todo-done`, `minutes-ai-note`; the items' own (`minutes-time`,
  `minutes-item-context`, `minutes-item-lines`, `minutes-photo`) and the Action Items' and Open Questions' cards
  (`minutes-card-actions`, `minutes-card-open`) keep their names. UI tests: `MinutesOverviewUITests` (the mock's
  `shape: overview`, `malformed` and `legacy`; light and dark, the default and the largest text size) beside
  `MinutesCardsUITests` (the four cards' baseline under `legacy`).

## File page (#5827)

Who it serves: people working with their computers' files from the phone or the browser (an Agent's Files tab, a computer's Files). Where it lives and what it does: docs/apple/ios.md "File page", docs/web/web.md "Side panels".

- **Path bar**: Back, Forward and Up as the page's circle buttons (`.themedButton()` clipped to a circle on iOS, `IconButton` on the web), then the path from the home (`Theme.Icon.home`) or `/`, every folder a button, the current one on `Theme.Fill.chip` (web `bg-field`), separated by `Theme.Icon.pathSeparator`. The session's folder carries a compact accent `Chip` "Session"; a folder outside the home ends with a compact `Chip` "Read Only" (`Theme.Icon.locked`). Why not the breadcrumb alone: the page now leaves the session's folder, so the path is the whole one and the history is a browser's.
- **Find field** under the path bar, beside the page's tool buttons: a capsule of `Theme.Fill.chip` at `Theme.Size.wellS` (the buttons' height) with the search mark and a clear button; the web's `SearchField` `sm`. Results replace the list while there are words in it.
- **Info**: a sheet with medium and large detents on iOS (a grouped list of `LabeledContent` rows, `.themedRow()`), a `Dialog` on the web, both with Copy Path; permissions in plain words before the code (`rw-r--r-- · 644`), the code in `Theme.Font.code`.
- **＋ menu** (#5828): the page's add button (`Theme.Icon.add`; web `plus`) opens New Folder (`Theme.Icon.newFolder`) and New Text File (`Theme.Icon.newFile`), then the uploads, each section apart; a name is asked in an alert on iOS (as Rename) and a `Dialog` on the web, the name before the extension selected.
- **Permissions** (#5831): a grid of system switches (iOS) or checkboxes (web), a row per owner / group / everyone else and a column per read / write / run, each switch labeled "who: what" for VoiceOver; Save is the confirmation action and asks in an alert or `Dialog` that names the change in words and codes (section 4.4, rung 2: not undoable from here).
- **Selection bar** (#5830): while selecting, five equal cells on a `.surface` card under the list, each a `ThemedIcon` over its word in `Theme.Font.label` (one line, scaling down to 80%), Trash in the danger color; disabled cells at 40%. The count sits where the find field was. Duplicate is `Theme.Icon.duplicate`, Compress `Theme.Icon.fileArchive`.
- **Editor** (#5829): full screen on iOS with Cancel and Save in the bar (Save the confirmation action), the title the file's name and "Edited" under it once changed; the keyboard bar's keys are the system's plain buttons with `Theme.Icon` symbols (`indent` is new). The web edits in place of the viewer with Cancel and a primary Save. The conflict asks in an alert (iOS) or a `Dialog` (web): Save as Copy first, Overwrite Anyway destructive and confirmed again.
- **Symlinks**: `Theme.Icon.symlink` on the icon's corner (accent), `Theme.Icon.error` in `Theme.danger` for a broken one, whose second line is the danger color too.

## Starting from what you already have (#5193)

Where a form sets something up that the user may already have working elsewhere (the web and iPhone provider wizards' first step),
the usual choices stay first and unchanged, and two more ways in follow them, each a row of the same list style:
**Paste a configuration**, and, only when there is something to show, a group **Found on your computers** listing what
the hub can import as it is, each row opening the import that already exists for it. The paste view is one monospace
text area with one plain line saying what it accepts and that nothing is saved until the user confirms; it is read by
itself 0.7 s after the text stops changing and at once on a paste, with the status line of "Checking typed credentials
by themselves" (a spinner while reading, then the result). What it found is a list of cards (a radio choice when there
are several): what it is, its address and how many models, then in secondary text what it brings along and what it
leaves out, by name and never by value. A draft that lands on something saved offers the existing choice as a radio
group (add its models to that provider, or replace only its key) beside adding a new one, and both open the existing
edit flow with the change filled in, so the user sees it before Save. Every part of the text that was not used gets a
line of its own (by line number or name), never dropped silently. The footer keeps one primary action, **Continue**,
enabled once something is chosen, with **Back** to the list. Reason: retyping an address, key and models the user
already has is slow and error-prone, and an import that fills the form instead of saving keeps the user in control
(principles 1 and 3); reusing the wizard's rows, status line, radio cards and the import dialog keeps one way to set a
provider up (principle 4). It adds no setting, style or motion.

An import link (#5420) is the same paste view opened by a link instead of typed into. Opening the link goes straight to
that view in its link mode: no text area, a row with the link icon saying it was opened from a link and, in secondary
text, that nothing is saved until Save, with **Paste something else** (the status line's action) to go back to the text
area. The link is read at once and its drafts, choices and notes look exactly like a paste's, so the wizard does not
grow a second way to review a provider; a refused link is one plain line about what is wrong with it, never its
content. Android uses this same link mode for `persona://provider/import` (#5428). iOS has no paste step, so the hub's
returned draft opens its existing connection form directly. On both phone apps the link itself is never shown and is
dropped when the wizard closes. Copying a link starts from the provider's dialog: a ⋯ `IconButton` before Edit opens a `Menu` (one row, **Copy
import link**), and the row opens a `Dialog` in place of the provider's (Edit does the same), with one line on what the
link brings, a `Switch` **Put a key in the link** that is off, and **Copy link** as the one primary button. Turning the
switch on widens what the link exposes, so, as in "Sharing a conversation publicly" (4.9), the key field comes with an
orange warning block (the warning mark, `--orange`) on where links travel, visible before Copy; the key is never
fetched from the hub and goes when the dialog closes. Reason: a link is how gateways and friends hand a setup over
(principle 1), the review and Save stay the user's (principle 3), and the risky part is opt-in and explained
(principle 2); it reuses the paste view, `Menu`, `Dialog`, `Switch` and the copy notice, and adds no setting, style or
motion.

## Project source on the web (#6570)

Audience: everyday users completing setup and chatting without promotional interruptions, and people who deliberately
look for the project's source. Settings > General ends in **About Persona** with one ordinary **Source code** row,
showing `github.com/nwverse/persona` as a link. Reuse `Group`, `Row` with its narrow-screen stacking, the existing
link color and touch-target class; no new token, button, animation, popup or preference. The link opens a new tab
only on activation. This project-information link is not a conversation link (4.6).

The web has no setup toast, empty-Agents pill or launch-count milestone asking for stars. The former browser-local
prompt state is ignored, not migrated or deleted; no hub data changes (principles 1–4 and 6).

## GitHub star prompts (#5761)

The Android client retains the three prompts below; iOS has a separate, unmerged implementation proposal (#5910).
The web removed them in #6570. The frozen Mac app, the watch and Vision have none. The phone rules are replayed from
`spec/fixtures/star-prompt`; state is device-local and never goes to the hub. A tap on **Star on GitHub** counts
as finished without verifying a star, and opens GitHub outside the app.

- **Empty Agents pill**: under the first-session empty state, with **Star on GitHub** and **Hide**. Hide permanently
  removes the pill on that device.
- **Milestone line**: initially after 20 new Agent starts. **Later** doubles the threshold past the current count
  and waits three days. It stays away while requests or sessions need the user, or a search is active.
- **Onboarding toast**: once per device on Android's setup **Done** (proposed iOS **Get started**), unless Star was
  already tapped. It uses the platform's timed toast and action.

## Knowledge map outline (#7251)

The web's knowledge map is a two-sided mind map; on the iPhone it is turned upright into an outline (`KnowledgeMapView`,
logic in `KBMap`), which the owner chose over a pinch-to-zoom map that, fitted to a phone, drew its words at 5–8 pt. Rows
are list rows: a topic (a dot in its branch's tone: accent, green, orange in turn), its subtopics and its points (the kind
as a `KBPill`, Pitfall and Open question in `Theme.caution`), each level indented by `Theme.Space.l` with a rail in the tone.
Folding uses the app's fold chevron (`Theme.Icon.disclosure`, turned when open, a 44 pt target) rather than the web's
round +/−, as every other fold on iOS. What starts open is the web's: the six biggest topics onto their subtopics, every
subtopic closed, more than three loose points beside subtopics under one closed Other. Tapping a point opens its sheet,
a topic or a project its page. From `Theme.Size.knowledgeMapTwoColumns` of width (any iPad in landscape, a 13-inch one upright beside its
sidebar) the outline sits beside a detail column for the selected node, the web map's side panel.
Text follows Dynamic Type (the rows are reading content) and every row reads as one VoiceOver element.
Android (#7678) draws the same outline with the same rules (`KnowledgeMapScreen`): rows of the design system's tokens, a dot and rails
in the branch's tone, a 48 dp fold target with Expanded / Collapsed read out, in all three styles, with no detail column.

## Repository link preview

Settings › Assistant › Repositories reuses the existing `Dialog`, form controls, inset preview and `Loading`.
A local parse of each link/provider change immediately replaces the provider, full path and canonical web root in
an announced status region. Invalid input clears that preview and disables Save; it never leaves the old identity
visible. The preview explicitly says remote existence is unconfirmed. Check and save is a separate explicit action,
keeps the draft on failure, and a browser timeout asks the user to refresh because persistence is unconfirmed.
This serves people binding a repository reference in advanced assistant settings; it adds no primary-screen
options, style, theme or motion. See `docs/assistant/managers.md` for provider semantics and verification boundaries.

A provider extension's source (#10330, `docs/extensions/repository-providers.md`) changes nothing on screen while
none is installed and no saved repository uses one: the list and the menu's three options stay as they were. The only
difference then is one more read, the sources beside the list (a 404 or a failure means the built-ins only).
Once there is one, the expanded area adds a **Sources** part, plain rows of each source's mark, name and either
"Built in" or its hosts in small mono text, with a neutral `Pill` **Unavailable** for one that is off. The provider
menu adds extensions by name after the built-ins, an off one disabled unless the edited repository uses it. The
preview of an extension's address waits for the source: the shared `Loading` line names it ("Reading the address
with Acme Code…"), then the inset preview names the source rather than its id; a host the source does not claim, an
unavailable source or its refusal (the hub's words) is one status line, and Save stays off until an identity
arrives. A refusal or failure is never final: a quiet **Try again** under it reads the address again, as does
editing the address. Until the sources are read (or when they can't be), the page claims no source is unavailable:
the repository being edited asks its own source through the hub, whose answer is shown. A saved repository of an
extension names its source under the address and, when that source is off or gone, adds the neutral **Unavailable**
pill and one `data-repository-note` line on what still works on this page (it stays saved and its details can be
edited). A source whose extension was renamed is still the saved repository's: the hub keeps the id it was saved
with, so the page matches it to the listed source with the same contribution that claims its host.

## Scheduled task execution options

The web task editor serves people scheduling specialist work as an optional **Execution options** group, closed by
default and placed before the prompt. Its title and configuration share one compact line, with ellipsis and a full
text tooltip when narrow. Its disclosure always shows the effective launcher name, agent backend,
model and thinking effort; repeated launcher/backend names are shown once. It reuses `Field`, native selects,
`ModelPicker`, semantic surface/text tokens and the existing dialog. Every untouched control explicitly follows
Assistant, with known inherited values shown. Selecting another launcher uses that agent's defaults for model and
effort. Empty choices retain dynamic inheritance rather than pinning a copied value. Task list rows use the same
formatter and resolved `execution` context, including inherited model and effort; older responses without that
context keep their legacy display and need no new fields. The selected computer is shown
inside the group. Launcher and model discovery use that computer's chat-compatible capabilities; the model catalog
is only requested when the group opens. A working directory
is an existing path on that computer, explained under the field. Switching execution context may start a new chat;
the hint says earlier chats are retained. Failed saves retain the draft, and saving/running disables the controls.
If Run now saves successfully but running fails, the open form accepts the saved execution pins and revision.
Fields edited while saving keep their newer input; untouched execution fields take the canonical saved values.
A later unrelated save stays sparse, and explicitly clearing a choice still restores inheritance.
Continuing the original chat requires empty task execution choices, including the computer and directory. The shared
form validation explains this and disables Save and Run now; clearing saved choices restores inheritance without
requiring that computer to be online. Toggling the original chat back on revalidates the current choices.
Launcher discovery failure or a missing saved launcher keeps the saved choice visible as unavailable, while the
Assistant inheritance option remains selectable. Selecting it explicitly clears launcher, model and effort together;
discovery alone never changes saved choices. Computer-scoped defaults come from that computer's launcher response.
A saved model alias uses its folded catalog row's capabilities and default thinking effort without rewriting the saved ID.
Defaults, model and launcher catalogs are keyed to the current assistant and their exact execution context; stale
responses cannot replace the current catalog. Discovery reads and the task save target the same assistant. Task
inheritance is requested explicitly, leaving ordinary Assistant picker defaults unchanged.
While model discovery is pending, the picker is disabled. A failed read leaves the saved selection visible and the
launcher inheritance recovery control usable, with no rows from the previous context.
Run history has a separate disclosure for its execution configuration, outside the button opening the conversation.
This is a secondary form extension; primary screens, themes and chat input remain unchanged.

## Optional repository associations

Coding-work metadata belongs in the Web to-do detail's **Repositories and
work** group; creation carries the same editor, folded, under its own optional
button. Ordinary to-dos still require only a title. On the detail the group is
read first and drawn in the existing grouped-row geometry: one row per saved
repository with its provider's own mark, project address and a `Pill` of its
link count, and one link row per issue or pull request. `RepositoryMark` reuses
the bundled GitHub asset and adds GitLab's mark without extending the unrelated
connector vocabulary. Repository names and addresses share one line; the
association store has no physical workspace path, so directory aliases are not
presented as filesystem paths. A link carries its complete source title as one
hyperlink, wrapping at narrow widths, followed by its number and an external
mark. An unknown title falls back to the localized kind and number. GitHub PRs
use the pull-request mark, GitLab MRs the merge mark and issues the existing
issue mark. A provider extension's repository uses the extension mark
(`Icon` `extension` in the secondary ink, `data-repository-mark="extension"`,
hidden from assistive technology; `git` would read as the branch mark of
**Source options** beside it). Its source's name, never the `ext:` id, follows
the address, so a narrow line cuts the name before the repository's path. Its
links open at their stored addresses, are numbered `#N` and take
the source's own `link_kinds` word for an untitled link. A pasted address of such
a repository is a pending link row ("Checked when you save") until the hub
reads it on save. When its source is off or uninstalled, the repository row adds
the neutral `Pill` **Unavailable** before its count and a `data-repository-note`
line beneath it; links keep opening and only adding one is turned off. The
picker shows the same mark and the source's name after the host. The
associations an agent keeps show without an editor. Editing is
the existing `Dialog` with a compact text button (**Manage links**), never an
always-open form. `TaskRepositoryBindingsEditor` is the controlled editor;
`TodoRepositoryBindings` owns existing-todo reads and writes and
`NewTodoRepositoryBindings` contributes a draft to the parent's single creation
request. These reuse `Group`, `Row`, `Pill`, `Button`, `IconButton`, `Menu`,
`Dialog`, `Collapse`, `SearchField`, `Loading`, `RowFailed`, shared settings
inputs, semantic tokens and toast Undo for draft removal. There is no new page
or workspace control.

Repositories are added through the search picker (name, path or host; ↑ ↓,
Home/End, Enter takes the first match, Escape closes and returns the focus to
its button), not a native `select`. Each repository's issues and PRs are link
rows with their own **Add issue or PR** field: Enter adds, Escape cancels, and a
URL of another repository is refused inline by `repositoryLinkView`, the same
rule the manager applies. Public HTTP aliases, scheme case and repository-path
case are normalized by the shared parser before that comparison; a self-hosted
instance's scheme and port must match exactly. Two links that normalize to the
same repository, kind and number are refused before create/save, with both
entries kept for correction. Directory names and source refs are validated with
the manager's rules before a create or save, and an invalid optional draft
shows the field error and cannot be submitted. Directory aliases and source
refs stay in the per-repository **Source options** disclosure; empty aliases
and default branches need no setup. Saved-source changes (`removed`, `identity_changed`) and
`unlisted` rows inferred from agent-added links are explained beneath their
repository while the saved source stays. A to-do's `pr_url` is drawn inside
this group as a link row with one line saying it predates the store, and never
beside it, so one place holds a task's issues and PRs. That row is a fallback
for a hub whose route is missing only: a successful store answer is
authoritative even when it is empty, because the manager imports the legacy
history once and honours an explicit removal, so an empty answer after the last
repository was removed must not resurrect the old PR.

Controls have labels, action groups wrap at narrow widths, and saving disables
edits. Failed saves keep input; conflicts offer explicit **Discard edits and
reload**, which reads the store again rather than reusing the stale draft, and
opening the editor always reads the newest store value and catalog because an
agent may have added a link. Manager/Todo/credential changes retire old results.
New copy ships in English, Simplified Chinese and Japanese. Evidence:
`web/src/ui/TaskRepositoryBindingsEditor.test.tsx` and
`web/e2e/todo-repositories-browser.py`.

## Lark topic group policy (#10238)

Settings › Connectors › Lark bot › Topic groups serves owners who want one group to require their explicit bot mention. It reuses Group, Row, Switch, Button, the native select and settings input styles; policy defaults off. Each group shows its saved switch. Enrollment allows choosing the policy before adding the group; a strict first enrollment stays separate from the home choice. The home select shows the effective home or the private-chat fallback. Both writes wait for the existing acknowledged save; failures preserve saved values and enrollment drafts. No primary chat screen or model setting changes.

## Shared botmux display setting (#10241)

For people using Agents, Web Settings › Agents reuses `Row`, `Switch`,
`SettingsHint` and `RowFailed` for **Show botmux sessions in Agents**. The row
shows the acknowledged shared value; a save disables the switch and shows Saving,
a failure keeps the last acknowledged value and offers Retry. A nearby Computers
link explains the independent per-computer inventory switch. The sidebar's
botmux chip is removed; its Lark group quick filter is absent when its count is
zero. Complete and paged lists exclude the same hosts before grouping and counts.
This requested interaction change adds no component, theme or motion pattern.
