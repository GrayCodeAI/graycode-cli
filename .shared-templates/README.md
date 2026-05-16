# Eco-Wide Shared Templates

Canonical source-of-truth templates for project meta-files used across every
hawk-eco repo. Copy these into a repo when adding it to the eco; refresh them
when updating eco-wide standards.

Files are named `<TARGET>.tmpl` where `<TARGET>` is the rendered filename. The
single placeholder `${PROJECT}` is replaced with the repo's short name (e.g.
`hawk`, `eyrie`, `tok`) when rendering.

Files in this directory:

- `editorconfig.tmpl` → rendered to `.editorconfig`
- `gitattributes.tmpl` → rendered to `.gitattributes`
- `CODE_OF_CONDUCT.md.tmpl` → rendered to `CODE_OF_CONDUCT.md`
- `SECURITY.md.tmpl` → rendered to `SECURITY.md`
- `CONTRIBUTING.md.tmpl` → rendered to `CONTRIBUTING.md`

To render manually:

```bash
PROJECT=hawk
for f in editorconfig gitattributes CODE_OF_CONDUCT.md SECURITY.md CONTRIBUTING.md; do
  src=".shared-templates/${f}.tmpl"
  case "$f" in
    editorconfig)   dst="${PROJECT}/.editorconfig"  ;;
    gitattributes)  dst="${PROJECT}/.gitattributes" ;;
    *)              dst="${PROJECT}/${f}"           ;;
  esac
  sed "s/\${PROJECT}/${PROJECT}/g" "$src" > "$dst"
done
```

Per-repo deviations should be minimised. If you must deviate, add a comment
at the top of the rendered file explaining why so the next refresh doesn't
silently revert it.
