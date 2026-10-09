/** OrgSelection, birim → bölüm → program seçimidir. Boş dize seçilmemiş demektir. */
export type OrgSelection = { facultyId: string; departmentId: string; programId: string }

export const emptyOrg: OrgSelection = { facultyId: '', departmentId: '', programId: '' }
