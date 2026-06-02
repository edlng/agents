/**
 * Custom assertion scoring function.
 * Factors cost into the final score — penalizes expensive responses
 * that don't proportionally improve quality.
 *
 * @param {Record<string, number>} namedScores - metric name → score (0-1)
 * @param {{ threshold?: number, tokensUsed?: { total: number, prompt: number, completion: number } }} context
 * @returns {{ pass: boolean, score: number, reason: string }}
 */
module.exports = function (namedScores, context) {
  const {
    accuracy = 0,
    accuracy_graded = 0,
    completeness = 0,
    completeness_rubric = 0,
    completeness_format = 0,
    rubric_quality = 0,
    rubric_reasoning = 0,
    cost = 1,
    cost_efficiency = 1,
    conciseness = 0,
  } = namedScores;

  // Aggregate quality score (weighted average of available quality metrics)
  const qualityMetrics = [
    { score: accuracy, weight: 3 },
    { score: accuracy_graded, weight: 2 },
    { score: completeness, weight: 3 },
    { score: completeness_rubric, weight: 2 },
    { score: completeness_format, weight: 1 },
    { score: rubric_quality, weight: 2 },
    { score: rubric_reasoning, weight: 2 },
    { score: conciseness, weight: 1 },
  ];

  // Only include metrics that were actually evaluated (score present in namedScores)
  const activeMetrics = qualityMetrics.filter((m) => m.score > 0);
  const totalWeight = activeMetrics.reduce((sum, m) => sum + m.weight, 0);
  const qualityScore = totalWeight > 0
    ? activeMetrics.reduce((sum, m) => sum + m.score * m.weight, 0) / totalWeight
    : 0;

  // Cost penalty: reduce score if cost assertions failed
  const costPenalty = (cost + cost_efficiency) / 2;
  const finalScore = qualityScore * 0.85 + costPenalty * 0.15;

  const threshold = context.threshold || 0.5;
  const pass = finalScore >= threshold;

  return {
    pass,
    score: finalScore,
    reason: `Quality: ${(qualityScore * 100).toFixed(1)}%, Cost factor: ${(costPenalty * 100).toFixed(1)}%, Final: ${(finalScore * 100).toFixed(1)}%`,
  };
};
