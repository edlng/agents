export type ClawdAction =
  | 'wave'
  | 'sunglasses'
  | 'laptop'
  | 'dance'
  | 'sleep'
  | 'coffee'
  | 'spin'
  | 'idea'
  | 'lgtm';

/** How long each action plays, in ms. Also drives the CSS timelines through --dur. */
export const ACTION_MS: Record<ClawdAction, number> = {
  wave: 2800,
  sunglasses: 4200,
  laptop: 6400,
  dance: 4200,
  sleep: 5000,
  coffee: 6000,
  spin: 1600,
  idea: 3200,
  lgtm: 3400,
};

export const ACTION_CAPTIONS: Record<ClawdAction, string> = {
  wave: 'hi there',
  sunglasses: 'deal with it',
  laptop: 'shipping code...',
  dance: 'tests are green',
  sleep: 'zzz...',
  coffee: 'refueling',
  spin: 'wheee',
  idea: 'got an idea!',
  lgtm: 'approved',
};

export const ACTIONS = Object.keys(ACTION_MS) as ClawdAction[];

export function pickAction(previous: ClawdAction | null): ClawdAction {
  const choices = ACTIONS.filter(a => a !== previous);
  return choices[Math.floor(Math.random() * choices.length)];
}
