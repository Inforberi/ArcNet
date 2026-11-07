import { Link } from 'react-router';
import { PageHeading, Panel } from '../shared/ui';

export default function NotFound() {
  return (
    <>
      <PageHeading title="Страница не найдена" description="Такого раздела в ArcNet нет." />
      <Panel title="Ошибка 404">
        <div className="panel-body">
          <p className="muted">Проверь адрес или вернись на главную страницу.</p>
          <Link className="button primary inline-action" to="/">Вернуться к обзору</Link>
        </div>
      </Panel>
    </>
  );
}
