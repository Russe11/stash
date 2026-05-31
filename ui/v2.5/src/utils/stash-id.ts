import { StashIdInput } from "src/core/generated-graphql";

// mergeStashIDs merges the src stash ID into the dest stash IDs.
// If the src stash ID is already in dest, the src stash ID overwrites the dest stash ID.
export function mergeStashIDs(dest: StashIdInput[], src: StashIdInput[]) {
  return dest
    .filter((i) => !src.find((j) => i.endpoint === j.endpoint))
    .concat(src);
}
