import { registryCol } from '../ui/registryTableColumns';

export function createMvpUi(t: (key: string) => string) {
  const contentColumns = [
    registryCol('Title', t('uiSystem.operations.title'), 'Title', 'left', '54%'),
    registryCol('SourceType', t('uiSystem.operations.source'), 'SourceType', 'left', '140px'),
    registryCol('Status', t('uiSystem.operations.state'), 'Status', 'left', '140px'),
  ];
  const supplierColumns = [
    registryCol('Name', t('uiSystem.operations.name'), 'Name', 'left', '220px'),
    registryCol('OpportunityID', t('uiSystem.operations.opportunity'), 'OpportunityID', 'left', '160px'),
    registryCol('PriceRange', t('uiSystem.operations.price'), 'PriceRange', 'left', '140px'),
    registryCol('Status', t('uiSystem.operations.state'), 'Status', 'left', '120px'),
    registryCol('URL', t('uiSystem.operations.link'), 'URL', 'left', '260px'),
  ];
  const sourceOptions = [
    { label: t('uiSystem.operations.allSources'), value: '' },
    { label: 'RSS', value: 'rss' },
    { label: t('uiSystem.operations.search'), value: 'search' },
    { label: t('uiSystem.operations.manualSource'), value: 'manual' },
  ];
  const contentStatusOptions = [
    { label: t('uiSystem.operations.allStates'), value: '' },
    { label: t('uiSystem.operations.candidate'), value: 'candidate' },
    { label: t('uiSystem.operations.selected'), value: 'selected' },
    { label: t('uiSystem.operations.reviewing'), value: 'in_review' },
    { label: t('uiSystem.operations.approved'), value: 'approved' },
    { label: t('uiSystem.operations.rejected'), value: 'rejected' },
  ];
  const audienceOptions = [
    { label: t('uiSystem.operations.all'), value: '' },
    { label: 'NovaTrade', value: 'novatrade_intel' },
    { label: t('uiSystem.operations.sourcing'), value: 'commerce_sourcing' },
  ];

  return { contentColumns, supplierColumns, sourceOptions, contentStatusOptions, audienceOptions };
}
