---
title: Jobs
layout: hextra-home
---

{{< hextra/hero-badge link="/documentation/Jobs/docs" >}}
  Documentation
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  A hierarchical task tracker&nbsp;<br class="sm:hx-block hx-hidden" />for the CLI
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  Plans that read like a spec, run like a DAG.&nbsp;<br class="sm:hx-block hx-hidden" />Built for agents, observed by humans.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-6">
{{< hextra/hero-button text="Read the docs" link="/documentation/Jobs/docs" >}}
</div>

<div class="hx-mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Agent-first ergonomics"
    subtitle="Output teaches as it answers. The CLI is the API."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(59,130,246,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Plans as data"
    subtitle="Author a plan in YAML, import in one transaction, run as a DAG with blockers and acceptance criteria."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(142,53,234,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Multi-agent ready"
    subtitle="Named identities, durable claims with TTL, and an event log so multiple agents can collaborate without stepping on each other."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(16,185,129,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Live dashboard"
    subtitle="`job serve` runs a read-only web UI with a subway-map dependency graph, so humans can watch agents work."
  >}}
  {{< hextra/feature-card
    title="Local-first"
    subtitle="One SQLite file. No SaaS, no auth, no daemon. Drop a binary on a laptop and you're done."
  >}}
  {{< hextra/feature-card
    title="Interruptible"
    subtitle="Pause anywhere, resume on a different model with fresh context. The plan is the memory."
  >}}
{{< /hextra/feature-grid >}}
