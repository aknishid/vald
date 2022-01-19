#ifdef __cplusplus
extern "C" {
#endif
  #include <stdio.h>
  #include <stdlib.h>

  typedef void* FaissQuantizer;
  typedef void* FaissIndex;
  typedef struct {
    FaissQuantizer  faiss_quantizer;
    FaissIndex      faiss_index;
  } FaissStruct;

  FaissStruct* faiss_create_index(
      const int d,
      const int nlist,
      const int m,
      const int nbits_per_idx);
  FaissStruct* faiss_read_index(const char* fname);
  void faiss_write_index(
      const FaissStruct* st,
      const char* fname);
  void faiss_train(
      const FaissStruct* st,
      const int nb,
      const float* xb);
  void faiss_add(
      const FaissStruct* st,
      const int nb,
      const float* xb,
      const long* xids);
  void faiss_search(
      const FaissStruct* st,
      const int k,
      const int nq,
      const float* xq,
      long* I,
      float* D);
  void faiss_remove(
      const FaissStruct* st,
      const int size,
      const long int* ids);
  void faiss_free(FaissStruct* st);
#ifdef __cplusplus
}
#endif
