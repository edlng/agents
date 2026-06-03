/**
 * Custom assertion scoring function.
 * Splits metrics into quality vs cost, blends 80/20.
 *
 * Cost metric: "cost" (output length proxy — fewer tokens = higher score)
 * Quality metrics: everything else (accuracy, completeness, rubric_quality, etc.)
 *
 * @param {Record<string, number>} namedScores - metric name → score (0-1)
 * @param {{ threshold?: number }} context
 * @returns {{ pass: boolean, score: number, reason: string }}
 */
module.exports = function (namedScores, context) {
  const costMetricNames = ['cost', 'conciseness'];

  const quality = [];
  const cost = [];

  for (const [name, score] of Object.entries(namedScores)) {
    if (costMetricNames.includes(name)) cost.push(score);
    else quality.push(score);
  }

  const avgQuality = quality.length > 0
    ? quality.reduce((a, b) => a + b, 0) / quality.length
    : 0;
  const avgCost = cost.length > 0
    ? cost.reduce((a, b) => a + b, 0) / cost.length
    : 1; // no cost signal = assume no penalty

  const finalScore = avgQuality * 0.8 + avgCost * 0.2;
  const pass = finalScore >= (context.threshold || 0.5);

  return {
    pass,
    score: finalScore,
    reason: `Quality: ${(avgQuality * 100).toFixed(0)}% | Cost: ${(avgCost * 100).toFixed(0)}%`,
  };
};
