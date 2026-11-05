import React, {type ErrorInfo, type ReactNode} from 'react';
import {Button, Notice, Panel} from '../shared/ui';

type Props = {children: ReactNode};
type State = {failed: boolean};

/** Isolates render and lazy-import failures instead of leaving an empty dashboard. */
export class PageErrorBoundary extends React.Component<Props, State> {
  state: State = {failed: false};

  static getDerivedStateFromError(): State {
    return {failed: true};
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // Do not send secrets, user content or stack traces to external services.
    console.error('ArcVPN page render failed', error.name, info.componentStack);
  }

  render(): ReactNode {
    if (!this.state.failed) return this.props.children;
    return (
      <main className="error-page">
        <Panel title="Не удалось открыть раздел">
          <div className="panel-body">
            <Notice>Ошибка загрузки интерфейса. VPN-соединения и настройки не изменены.</Notice>
            <Button variant="primary" onClick={() => window.location.reload()}>Перезагрузить страницу</Button>
          </div>
        </Panel>
      </main>
    );
  }
}
