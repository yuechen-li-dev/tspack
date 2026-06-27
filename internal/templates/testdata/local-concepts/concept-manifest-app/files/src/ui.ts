import clsx from "clsx";

export function cx(...classNames: Array<string | false | null | undefined>) {
  return clsx(classNames);
}
