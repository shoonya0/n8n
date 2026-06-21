# Skills Directory

AI agent skills for Cursor. Each skill provides structured instructions that the AI follows when triggered by matching user intent.

## Layout

```
.cursor/skills/
  analyze-production-logs/     # Project-specific: parse dm-enquer-go.log JSONL
  dev-deploy-feedback/         # Project-specific: code → deploy → logs → iterate
  cc-skills-golang/            # Third-party plugin: samber/cc-skills-golang v1.2.2
    .cursor-plugin/plugin.json #   Cursor discovers skills via "skills": "./skills/"
    skills/
      golang-benchmark/        #   35 Go skill modules
      golang-cli/
      golang-code-style/
      golang-concurrency/
      golang-context/
      ...
```

## Skill Types

### Project-specific skills (top-level folders)

| Skill | Trigger |
|---|---|
| `analyze-production-logs` | "analyze logs", "check production health", "what's happening in prod" |
| `dev-deploy-feedback` | "deploy", "push to prod", "make changes and deploy" |

### Go language skills (cc-skills-golang plugin)

35 Go skills covering: code style, concurrency, context, testing, benchmarking, security, error handling, design patterns, observability, database, gRPC, linting, performance, project layout, and library-specific guidance (samber/lo, samber/mo, testify, etc.).

These are discovered by Cursor via `.cursor-plugin/plugin.json` in the `cc-skills-golang` directory.

## How Skills Work

1. Cursor loads skill descriptions at startup (~100 tokens each)
2. When user intent matches a description, the full SKILL.md is loaded
3. Skills may reference secondary files in `references/` for detailed docs
4. Skills with checklists should be followed step-by-step

## Adding New Project Skills

Create a new folder under `.cursor/skills/`:

```
.cursor/skills/my-new-skill/
  SKILL.md    # Required: YAML frontmatter (name, description) + instructions
```

Frontmatter format:
```yaml
---
name: my-new-skill
description: "When to trigger this skill. Be specific about user intent."
---
```

## Updating Third-Party Skills

The `cc-skills-golang` plugin was cloned from `github.com/samber/cc-skills-golang`. To update:

```bash
cd /tmp
git clone https://github.com/samber/cc-skills-golang.git
rm -rf /path/to/project/.cursor/skills/cc-skills-golang/skills
cp -r cc-skills-golang/skills /path/to/project/.cursor/skills/cc-skills-golang/
```
