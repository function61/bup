import { DocLink } from 'component/doclink';
import { volumeAutocomplete } from 'component/autocompletes';
import { thousandSeparate } from 'component/numberformatter';
import { Result } from 'f61ui/component/result';
import { Info } from 'f61ui/component/info';
import {
	SuccessLabel,
	WarningLabel,
	DangerLabel,
	DefaultLabel,
	Panel,
	tableClassStripedHover,
} from 'f61ui/component/bootstrap';
import { CommandButton, CommandLink } from 'f61ui/component/CommandButton';
import { Dropdown } from 'f61ui/component/dropdown';
import { shouldAlwaysSucceed } from 'f61ui/utils';
import {
	DatabaseDiscoverReconcilableReplicationPolicies,
	DatabaseReconcileReplicationPolicy,
	ReplicationpolicyCreate,
	ReplicationpolicyRename,
	ReplicationpolicyChangeDesiredVolumes,
	ReplicationpolicyChangeMinZones,
} from 'generated/stoserver/stoservertypes_commands';
import {
	getReconcilableItems,
	getReplicationPolicies,
	getVolumes,
} from 'generated/stoserver/stoservertypes_endpoints';
import {
	ReconciliationReport,
	ReplicationPolicy,
	DocRef,
	Volume,
} from 'generated/stoserver/stoservertypes_types';
import { SettingsLayout } from 'layout/settingslayout';
import * as React from 'react';

interface ReplicationPoliciesPageState {
	selectedCollIds: string[];
	replicationpolicies: Result<ReplicationPolicy[]>;
	reconciliationReport: Result<ReconciliationReport>;
	volumes: Result<Volume[]>;
}

export default class ReplicationPoliciesPage extends React.Component<
	{},
	ReplicationPoliciesPageState
> {
	state: ReplicationPoliciesPageState = {
		selectedCollIds: [],
		reconciliationReport: new Result<ReconciliationReport>((_) => {
			this.setState({ reconciliationReport: _ });
		}),
		replicationpolicies: new Result<ReplicationPolicy[]>((_) => {
			this.setState({ replicationpolicies: _ });
		}),
		volumes: new Result<Volume[]>((_) => {
			this.setState({ volumes: _ });
		}),
	};

	componentDidMount() {
		shouldAlwaysSucceed(this.fetchData());
	}

	componentWillReceiveProps() {
		shouldAlwaysSucceed(this.fetchData());
	}

	render() {
		return (
			<SettingsLayout title="Replication policies" breadcrumbs={[]}>
				<Panel
					heading={
						<div>
							Replication policies &nbsp;
							<DocLink doc={DocRef.DocsUsingReplicationPoliciesIndexMd} />
							&nbsp;
							<Dropdown>
								<CommandLink command={ReplicationpolicyCreate()} />
							</Dropdown>
						</div>
					}>
					{this.renderPolicies()}
				</Panel>

				<Panel heading="Reconciliation">{this.renderReconcilable()}</Panel>
			</SettingsLayout>
		);
	}

	private renderPolicies() {
		const [replicationpolicies, volumes, loadingOrError] = Result.unwrap2(
			this.state.replicationpolicies,
			this.state.volumes,
		);

		if (!replicationpolicies || !volumes) {
			return loadingOrError;
		}

		return (
			<table className={tableClassStripedHover}>
				<thead>
					<tr>
						<th>Name</th>
						<th>
							New data goes to{' '}
							<Info text="Old data stays where it was written, except if you increase the replica count (derived from these volumes), old data will also be replicated to satisfy policy." />
						</th>
						<th>
							Replica count{' '}
							<Info text="This is derived from count of 'New data goes to' volumes" />
						</th>
						<th>
							Zones{' '}
							<Info text="Minimum amount of separate physical zones data should be stored in. This protects from fires, flooding etc." />
						</th>
						<th>
							Data safety <Info text="Calculated from replica count &amp; zones" />
						</th>
						<th />
					</tr>
				</thead>
				<tbody>
					{replicationpolicies.map((rp) => (
						<tr key={rp.Id}>
							<td title={`Id= ${rp.Id}`}>{rp.Name}</td>
							<td>
								{rp.DesiredVolumes.map((id) => {
									const vols = volumes.filter((v) => v.Id === id);

									const volLabel = vols[0] ? vols[0].Label : '(error)';

									return (
										<span className="margin-right">
											<DefaultLabel>{volLabel}</DefaultLabel>
										</span>
									);
								})}
							</td>
							<td>{replicaCount(rp)}</td>
							<td>{rp.MinZones}</td>
							<td>{this.dataSafety(replicaCount(rp), rp.MinZones)}</td>
							<td>
								<Dropdown>
									<CommandLink
										command={ReplicationpolicyRename(rp.Id, rp.Name)}
									/>
									<CommandLink
										command={ReplicationpolicyChangeDesiredVolumes(
											rp.Id,
											{
												Volume1: volumeAutocomplete,
												Volume2: volumeAutocomplete,
												Volume3: volumeAutocomplete,
												Volume4: volumeAutocomplete,
											},
											{ disambiguation: rp.Name },
										)}
									/>
									<CommandLink
										command={ReplicationpolicyChangeMinZones(
											rp.Id,
											rp.MinZones,
											{ disambiguation: rp.Name },
										)}
									/>
								</Dropdown>
							</td>
						</tr>
					))}
				</tbody>
			</table>
		);
	}

	private dataSafety(replCount: number, minZones: number) {
		if (replCount < 2) {
			return (
				<DangerLabel>
					Data loss very likely{' '}
					<Info text="Stored on one volume only (see replica count)" />
				</DangerLabel>
			);
		}

		if (minZones < 2) {
			return (
				<WarningLabel>
					Data loss possible{' '}
					<Info text="Fire, flood etc. can destroy your data (see zone count)" />
				</WarningLabel>
			);
		}

		return (
			<SuccessLabel>
				Data is pretty safe{' '}
				<Info text="Stored in multiple physical zones to protect from fire, floods etc." />
			</SuccessLabel>
		);
	}

	private renderReconcilable() {
		const [report, volumes, loadingOrError] = Result.unwrap2(
			this.state.reconciliationReport,
			this.state.volumes,
		);

		if (!report || !volumes) {
			return loadingOrError;
		}

		const masterCheckedChange = () => {
			const selectedCollIds = report.Items.map((item) => item.CollectionId);

			this.setState({ selectedCollIds });
		};

		const collCheckedChange = (e: React.ChangeEvent<HTMLInputElement>) => {
			const collId = e.target.value;

			// removes collId if it already exists
			const selectedCollIds = this.state.selectedCollIds.filter((id) => id !== collId);

			if (e.target.checked) {
				selectedCollIds.push(collId);
			}

			this.setState({ selectedCollIds });
		};

		return (
			<div>
				<p>
					{thousandSeparate(report.TotalItems)} collections in non-compliance with its
					replication policy.
				</p>

				<table className={tableClassStripedHover}>
					<thead>
						<tr>
							<th>
								<input type="checkbox" onChange={masterCheckedChange} />
							</th>
							<th>Collection</th>
							<th>Blobs</th>
							<th>Problems</th>
							<th colSpan={2}>
								Replicas
								<br />
								Desired &nbsp; &nbsp;Reality
							</th>
						</tr>
					</thead>
					<tbody>
						{report.Items.map((r) => (
							<tr>
								<td>
									<input
										type="checkbox"
										checked={
											this.state.selectedCollIds.indexOf(r.CollectionId) !==
											-1
										}
										onChange={collCheckedChange}
										value={r.CollectionId}
									/>
								</td>
								<td>{r.Description}</td>
								<td>{thousandSeparate(r.TotalBlobs)}</td>
								<td>
									{r.ProblemRedundancy && <DangerLabel>redundancy</DangerLabel>}
									{r.ProblemZoning && <DangerLabel>zoning</DangerLabel>}
								</td>
								<td>{r.DesiredReplicaCount}</td>
								<td>
									{r.ReplicaStatuses.sort(
										(a, b) => b.BlobCount - a.BlobCount,
									).map((rs) => {
										const vol = volumes.filter((v) => v.Id === rs.Volume);
										const volLabel =
											vol.length === 1 ? vol[0].Label : '(error)';

										if (rs.BlobCount === r.TotalBlobs) {
											return (
												<span className="margin-right">
													<DefaultLabel
														title={
															rs.BlobCount.toString() + ' blob(s)'
														}>
														{volLabel}
													</DefaultLabel>
												</span>
											);
										} else {
											return (
												<span className="margin-right">
													<WarningLabel
														title={
															rs.BlobCount.toString() + ' blob(s)'
														}>
														{volLabel}
													</WarningLabel>
												</span>
											);
										}
									})}
								</td>
							</tr>
						))}
					</tbody>
					<tfoot>
						<tr>
							<td colSpan={2}>
								{this.state.selectedCollIds.length > 0 && (
									<div>
										<CommandButton
											command={DatabaseReconcileReplicationPolicy(
												this.state.selectedCollIds.join(','),
												{ Volume: volumeAutocomplete },
											)}
										/>
									</div>
								)}
							</td>
							<td colSpan={99}>
								{thousandSeparate(
									report.Items.reduce(
										(prev, current) => prev + current.TotalBlobs,
										0,
									),
								)}
							</td>
						</tr>
					</tfoot>
				</table>

				<CommandButton command={DatabaseDiscoverReconcilableReplicationPolicies()} />
			</div>
		);
	}

	private async fetchData() {
		this.state.replicationpolicies.load(() => getReplicationPolicies());
		this.state.reconciliationReport.load(() => getReconcilableItems());
		this.state.volumes.load(() => getVolumes());
	}
}

function replicaCount(policy: ReplicationPolicy): number {
	return policy.DesiredVolumes.length;
}
