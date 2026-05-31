import React, { useMemo } from "react";
import { StashId } from "src/core/generated-graphql";

export type LinkType = "performers" | "scenes" | "studios" | "tags";

export const StashIDPill: React.FC<{
  stashID: Pick<StashId, "endpoint" | "stash_id">;
  linkType: LinkType;
}> = ({ stashID, linkType }) => {
  const { endpoint, stash_id } = stashID;

  const endpointName = useMemo(() => {
    return endpoint;
  }, [endpoint]);

  return (
    <span className="stash-id-pill" data-endpoint={endpointName}>
      <span>{endpointName}</span>
      <span>{stash_id}</span>
    </span>
  );
};

interface IStashIDsField {
  values: StashId[];
  linkType: LinkType;
}

export const StashIDsField: React.FC<IStashIDsField> = ({
  values,
  linkType,
}) => {
  if (!values.length) return null;

  return (
    <ul className="pl-0 mw-100">
      {values.map((v) => (
        <li key={v.stash_id} className="row no-gutters">
          <StashIDPill linkType={linkType} stashID={v} />
        </li>
      ))}
    </ul>
  );
};
