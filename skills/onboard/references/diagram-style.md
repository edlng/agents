# Architecture Diagram Contract

Architecture diagrams are evidence views, not decoration. Every onboarding
guide must include at least one icon-based, self-contained architecture
diagram. Mermaid and the AWS Diagram MCP server are prohibited.

## Evidence-First Inventory

Create a row for every proposed node and edge:

| ID | Kind | Label | Evidence status | Evidence | Include? |
| --- | --- | --- | --- | --- | --- |
| D-01 | Node | `<component>` | Source-backed | `<path and symbol>` | Yes |
| D-02 | Edge | `<interaction>` | Verified | `<command/result or trace>` | Yes |

Every node, boundary, connector, protocol, direction, and sequence number must
be supported by **Verified** or **Source-backed** evidence, or visibly labeled
**Inferred** or **Unknown**. Omit unsupported detail. Keep evidence IDs in the
editable source or adjacent prose so the rendered diagram remains auditable.

## Required Renderer Policy

Use the first applicable path:

1. **diagrams.net.** When `drawio` or the diagrams.net desktop CLI is already
   installed, create editable `.drawio` source and export a self-contained SVG.
   Record the observed renderer version and exact export command.
2. **Direct SVG fallback.** Otherwise author the SVG directly. Embed icon paths,
   fonts, fills, markers, and styles in the SVG. Use the bundled icons under
   `references/icons/`; the SVG itself is the editable source. This path is
   always available because it needs only text-file writing.

Do not use Mermaid, even if the host supports it or deadlines make it tempting.
Do not invoke, recommend, probe, or wait for AWS Diagram MCP. Do not skip the
diagram because diagrams.net or provider-specific icons are unavailable. Other
renderers are used only when the user explicitly requests one.

Do not install a renderer, icon package, plugin, runtime, or dependency without
direct user approval. Temporary, isolated, ignored, or virtual-environment
installation is still installation.

## Icon Policy

- Use the bundled Lucide subset for custom applications, users, processes,
  protocols, generic infrastructure, data, security, and unknown components.
  It is pinned to the source commit documented in
  `references/icons/MANIFEST.md`; preserve `references/icons/LICENSE`.
- Use official provider service icons only when current evidence proves the
  service and the icon is already available or can be retrieved without
  installation. Record source, version or retrieval date, license or usage
  terms, and SHA-256.
- If an official provider icon is unavailable, use the closest bundled generic
  icon and an accurate text label. Never omit the diagram or substitute a
  visually similar provider service.
- Use one neutral icon family per diagram. Do not mix unrelated line-icon
  families. Provider icons may retain their official colors.
- Final SVGs must not depend on remote image URLs, external fonts, custom
  Obsidian CSS, community plugins, or runtime JavaScript.

## Visual Grammar

- Draw explicit system, trust, network, process, and ownership boundaries only
  when evidence proves them.
- Lay out the primary flow left to right. Use orthogonal, labeled, directional
  connectors and avoid crossings.
- Number interactions when order matters. Match each number to concise
  accessible prose below the diagram.
- Group components by proven responsibility or boundary, not merely by folder.
- Give each major node one icon, a short title, and at most two compact detail
  lines. Use details in prose instead of shrinking diagram text.
- Use restrained neutral surfaces, one accent for custom components, and
  provider colors only for official service icons. Maintain readable contrast
  on both light and dark hosts by giving the SVG an explicit background.
- Include a compact legend only when boundaries, evidence states, or connector
  styles need explanation.
- Keep labels readable at normal note width. Avoid nested cards, decorative
  gradients, heavy shadows, excessive rounded containers, and line crossings.

## Output Placement

Store source and output under `Projects/<repository>/assets/`:

- diagrams.net path: `<slug>-architecture.drawio` and
  `<slug>-architecture.svg`
- direct fallback: `<slug>-architecture.svg`

Embed the SVG as an image reference. Do not paste raw `<svg>` markup into
Markdown because host CSS can override generic SVG classes. If the persistence
tool cannot write non-note assets, embed the complete SVG as a base64 data URI
in an `<img>` tag and keep editable SVG source in a collapsed code block. Put
the high-level diagram in the main onboarding note and detailed diagrams with
their subsystem notes. Provide accessible prose for every diagram.

## Validation Checklist

- Every node and connector maps to inventory evidence.
- Node icons match their labels; provider icons represent proven services only.
- Arrow direction and numbered order match the traced behavior and prose.
- Boundaries are evidenced, named, visually distinct, and not misleading.
- Labels, icons, connectors, and sequence numbers are readable at normal note
  width with no clipping, overlap, or crossing through text.
- SVG is well-formed XML, has a finite `viewBox`, explicit background, accessible
  title/description, and no remote dependencies or scripts.
- diagrams.net source opens and exports successfully when that path is used.
- When a rasterizer or GUI is available, inspect a fresh render for nonblank
  output, framing, overlap, and light/dark-host readability. If unavailable,
  state that visual verification was not observed.
- The note discloses renderer and version, source/output paths, icon family,
  provider-icon source, fallbacks, and unresolved rendering limitations.
- Mermaid syntax and AWS Diagram MCP references are absent from generated notes
  and diagram disclosures.

## References

- diagrams.net: https://github.com/jgraph/drawio
- Lucide: https://github.com/lucide-icons/lucide
- AWS Architecture Icons: https://aws.amazon.com/architecture/icons/
- AWS Reference Architecture Diagrams:
  https://aws.amazon.com/architecture/reference-architecture-diagrams/
