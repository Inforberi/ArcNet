import {lazy, Suspense, useState} from 'react';
import {Button, Loading, PageHeading} from '../shared/ui';
import RoutingProfiles from '../features/routing-profiles/RoutingProfiles';
const RoutePaths = lazy(() => import('../features/routing/RoutePaths'));

export default function Routing() {
  const [tab, setTab] = useState<'rules' | 'paths'>('rules');
  return <>
    <PageHeading title="Маршруты" description="Настройте правила трафика. Каскады серверов доступны отдельно."/>
    <div className="routing-page-tabs" role="group" aria-label="Раздел маршрутизации">
      <Button aria-pressed={tab === 'rules'} variant={tab === 'rules' ? 'primary' : 'default'} onClick={() => setTab('rules')}>Правила трафика</Button>
      <Button aria-pressed={tab === 'paths'} variant={tab === 'paths' ? 'primary' : 'default'} onClick={() => setTab('paths')}>Серверы и каскады</Button>
    </div>
    {tab === 'rules' ? <RoutingProfiles/> : <Suspense fallback={<Loading/>}><RoutePaths/></Suspense>}
  </>;
}
