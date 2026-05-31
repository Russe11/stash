import { SlimSceneDataFragment } from "src/core/generated-graphql";
import { IScrapedScene } from "../context";

export function minDurationDiff(
  stashScene: SlimSceneDataFragment,
  duration: number
) {
  let ret = 9999;
  stashScene.files.forEach((cv) => {
    if (ret === 0) return;

    const d = Math.abs(duration - cv.duration);
    if (d < ret) {
      ret = d;
    }
  });

  return ret;
}

export function calculateDurationComparisonScore(
  stashScene: SlimSceneDataFragment,
  scrapedScene: IScrapedScene
) {
  if (scrapedScene.duration) {
    const diff = minDurationDiff(stashScene, scrapedScene.duration);
    const match = diff <= 5 ? 1 : 0;
    return [match, match, diff];
  }

  return [0, 0, 0];
}

export function compareScenesForSort(
  stashScene: SlimSceneDataFragment,
  sceneA: IScrapedScene,
  sceneB: IScrapedScene
) {
  const [
    nbDurationMatchSceneA,
    ratioDurationMatchSceneA,
    minDurationDiffSceneA,
  ] = calculateDurationComparisonScore(stashScene, sceneA);
  const [
    nbDurationMatchSceneB,
    ratioDurationMatchSceneB,
    minDurationDiffSceneB,
  ] = calculateDurationComparisonScore(stashScene, sceneB);

  if (nbDurationMatchSceneA != nbDurationMatchSceneB) {
    return nbDurationMatchSceneB - nbDurationMatchSceneA;
  }

  // Same number of phash & duration, check duration ratio
  if (ratioDurationMatchSceneA != ratioDurationMatchSceneB) {
    return ratioDurationMatchSceneB - ratioDurationMatchSceneA;
  }

  // fall back to duration difference - less is better
  return minDurationDiffSceneA - minDurationDiffSceneB;
}
