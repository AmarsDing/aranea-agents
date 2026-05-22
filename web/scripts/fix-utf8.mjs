import fs from "fs";

const fixes = [
  {
    path: "src/features/cron/useCronTasksPage.ts",
    apply(s) {
      return s
        .split("\n")
        .map((line) => {
          if (line.includes('schedule_type === "once"')) {
            return '    if (cfg.schedule_type === "once") return `once @ ${formatDate(cfg.run_at)}`;';
          }
          if (line.includes('cfg.target_type === "team"')) {
            return '    if (cfg.target_type === "team" || cfg.team_id) return `Team: ${teamLabel(cfg.team_id || "")}`;';
          }
          if (line.includes("return `Agent") && line.includes("agentLabel")) {
            return "    return `Agent: ${agentLabel(row.agent_id)}`;";
          }
          if (line.includes("if (!value) return")) {
            return '    if (!value) return "-";';
          }
          return line;
        })
        .join("\n");
    }
  },
  {
    path: "src/features/session/useSessionTurnsPanel.ts",
    apply(s) {
      return s.replace(
        /pageLabel = computed\([\s\S]*?\);/,
        `pageLabel = computed(
    () => \`\${offset.value + 1}-\${Math.min(offset.value + PAGE_SIZE, total.value)} / \${total.value}\`,
  );`
      );
    }
  }
];

for (const { path, apply } of fixes) {
  const raw = fs.readFileSync(path, "utf8");
  const next = apply(raw);
  fs.writeFileSync(path, next, "utf8");
  const bad = [...next].filter((c) => c === "\uFFFD").length;
  console.log(path, "fffd=", bad);
}
