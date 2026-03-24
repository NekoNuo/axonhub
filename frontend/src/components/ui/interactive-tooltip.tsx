import * as React from 'react';
import { Tooltip, TooltipContent, TooltipTrigger } from './tooltip';

type PointerEventHandler = React.PointerEventHandler<HTMLElement>;
type FocusEventHandler = React.FocusEventHandler<HTMLElement>;
type MouseEventHandler = React.MouseEventHandler<HTMLElement>;

function composeEventHandlers<E>(
  originalHandler: ((event: E) => void) | undefined,
  nextHandler: (event: E) => void
) {
  return (event: E) => {
    originalHandler?.(event);
    nextHandler(event);
  };
}

interface InteractiveTooltipProps extends Omit<React.ComponentProps<typeof TooltipContent>, 'children'> {
  children: React.ReactElement<React.HTMLAttributes<HTMLElement>>;
  content: React.ReactNode;
}

export function InteractiveTooltip({ children, content, ...contentProps }: InteractiveTooltipProps) {
  const [open, setOpen] = React.useState(false);

  const trigger = React.cloneElement(children, {
    onMouseEnter: composeEventHandlers(children.props.onMouseEnter as MouseEventHandler | undefined, () => {
      setOpen(true);
    }),
    onMouseLeave: composeEventHandlers(children.props.onMouseLeave as MouseEventHandler | undefined, () => {
      setOpen(false);
    }),
    onFocus: composeEventHandlers(children.props.onFocus as FocusEventHandler | undefined, () => {
      setOpen(true);
    }),
    onBlur: composeEventHandlers(children.props.onBlur as FocusEventHandler | undefined, () => {
      setOpen(false);
    }),
    onPointerDown: composeEventHandlers(children.props.onPointerDown as PointerEventHandler | undefined, (event) => {
      if (event.pointerType !== 'mouse') {
        setOpen((prev) => !prev);
      }
    }),
  });

  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent {...contentProps}>{content}</TooltipContent>
    </Tooltip>
  );
}
