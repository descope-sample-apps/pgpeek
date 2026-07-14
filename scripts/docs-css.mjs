const criticalBlockPattern =
  /\/\* critical-css:start \*\/([\s\S]*?)\/\* critical-css:end \*\//g;

export function splitCriticalCss(source) {
  const criticalBlocks = [];
  const deferred = source.replace(criticalBlockPattern, (_match, block) => {
    criticalBlocks.push(block.trim());
    return "";
  });

  if (criticalBlocks.length === 0) {
    throw new Error("docs/styles.css has no critical CSS blocks");
  }

  return {
    critical: `${criticalBlocks.join("\n\n")}\n`,
    deferred: `${deferred.replace(/\n{3,}/g, "\n\n").trim()}\n`,
  };
}
