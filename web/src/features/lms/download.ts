import { toast } from 'sonner'

import { useAppDispatch } from '@/app/hooks'
import { agoraApi, type DownloadUrl } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { openDownload } from '@/shared/lib/download'

/** useSignedDownload, imzalı URL'yi alıp dosyayı indirir (tarayıcı belleğine alınmaz). */
export function useSignedDownload() {
  const message = useErrorMessage()
  return async (fetchUrl: () => Promise<DownloadUrl>) => {
    try {
      openDownload((await fetchUrl()).url)
    } catch (e) {
      toast.error(message(e as never))
    }
  }
}

/** useDownload, etkinliğin dosyasını indirir; öğrencinin indirdiği dosya tamamlanır, ders alanı yenilenir. */
export function useDownload() {
  const dispatch = useAppDispatch()
  const download = useSignedDownload()
  return async (activityId: string, fileId: string) => {
    await download(() =>
      dispatch(
        agoraApi.endpoints.getActivityFileUrl.initiate(
          { id: activityId, fileId },
          { subscribe: false, forceRefetch: true },
        ),
      ).unwrap(),
    )
    dispatch(agoraApi.util.invalidateTags(['Ders alanı']))
  }
}

/**
 * useSubmissionDownload, bir teslim dosyasını indirir: userId verilmezse öğrencinin kendi
 * teslimi, verilirse öğretim elemanının gördüğü öğrenci teslimi.
 */
export function useSubmissionDownload(activityId: string, userId?: string) {
  const dispatch = useAppDispatch()
  const download = useSignedDownload()
  return (fileId: string) =>
    download(() =>
      (userId
        ? dispatch(
            agoraApi.endpoints.getStudentSubmissionFileUrl.initiate(
              { id: activityId, userId, fileId },
              { subscribe: false, forceRefetch: true },
            ),
          )
        : dispatch(
            agoraApi.endpoints.getMySubmissionFileUrl.initiate(
              { id: activityId, fileId },
              { subscribe: false, forceRefetch: true },
            ),
          )
      ).unwrap(),
    )
}
